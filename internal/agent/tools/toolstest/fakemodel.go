package toolstest

import (
	"context"
	"errors"
	"sync"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// FakeStep 脚本步骤:toolName 非空 → 本轮发起工具调用;否则输出最终文本。
type FakeStep struct {
	ToolName string
	ToolArgs string
	Content  string
}

// FakeModel 脚本化假模型,每次 Stream/Generate 弹出一个步骤。
type FakeModel struct {
	mu    sync.Mutex
	steps []FakeStep
	// Hold 非 nil 时,出脚本前阻塞等待(用于并发上限等时序测试)。
	Hold chan struct{}
}

func NewFakeModel() *FakeModel { return &FakeModel{} }

func (f *FakeModel) Append(s FakeStep) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.steps = append(f.steps, s)
}

func (f *FakeModel) pop(ctx context.Context) (FakeStep, bool, error) {
	f.mu.Lock()
	hold := f.Hold
	f.mu.Unlock()
	if hold != nil {
		// 时序测试:模型挂起期间信号量被占住;与真实模型一样响应 ctx 取消
		select {
		case <-hold:
		case <-ctx.Done():
			return FakeStep{}, false, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.steps) == 0 {
		return FakeStep{}, false, nil
	}
	s := f.steps[0]
	f.steps = f.steps[1:]
	return s, true, nil
}

func (f *FakeModel) Generate(ctx context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	step, ok, err := f.pop(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("fake: 脚本耗尽")
	}
	if step.ToolName != "" {
		return &schema.Message{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{{
				ID:       "call-1",
				Type:     "function",
				Function: schema.FunctionCall{Name: step.ToolName, Arguments: step.ToolArgs},
			}},
		}, nil
	}
	return &schema.Message{Role: schema.Assistant, Content: step.Content}, nil
}

func (f *FakeModel) Stream(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	sr, sw := schema.Pipe[*schema.Message](2)
	go func() {
		defer sw.Close()
		msg, err := f.Generate(ctx, in, opts...)
		if err != nil {
			sw.Send(nil, err)
			return
		}
		sw.Send(msg, nil)
	}()
	return sr, nil
}
