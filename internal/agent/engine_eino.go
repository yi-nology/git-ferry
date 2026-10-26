package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/oekit/concurrency/async"

	"github.com/yi-nology/git-ferry/internal/agent/engine"
	"github.com/yi-nology/git-ferry/internal/agent/engine/delegate"
	"github.com/yi-nology/git-ferry/internal/agent/tools"
)

// einoEngine 内置后端:eino ChatModelAgent + 工具装饰器。
//
// 工具在**本进程**执行(直调 core service),因此它拥有完整的
// 「危险工具确认」链路;外部 coding agent 后端没有这个能力。
type einoEngine struct {
	memManifest string
	runner      *adk.Runner
	reg         *tools.Registry
	modelName   string
}

// NewEinoEngine 生产构造:OpenAI 兼容模型(BaseURL 可指内网 vLLM/Ollama)。
func NewEinoEngine(cfg *Config, apiKey string, svc tools.SyncService) (engine.Engine, error) {
	temp := float32(cfg.Temperature)
	maxTokens := cfg.MaxTokens
	cm, err := openai.NewChatModel(context.Background(), &openai.ChatModelConfig{
		APIKey:      apiKey,
		BaseURL:     cfg.BaseURL,
		Model:       cfg.Model,
		Temperature: &temp,
		MaxTokens:   &maxTokens,
		Timeout:     time.Duration(cfg.TimeoutSeconds) * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("构建模型失败: %w", err)
	}
	return NewEinoEngineWithModel(cfg, cm, tools.NewRegistry(svc))
}

// NewEinoEngineWithModel 以注入模型构造(测试用)。
func NewEinoEngineWithModel(cfg *Config, cm model.BaseModel[*schema.Message], reg *tools.Registry) (engine.Engine, error) {
	agent, err := adk.NewChatModelAgent(context.Background(), &adk.ChatModelAgentConfig{
		Name:          "git-sync-assistant",
		Description:   "仓库同步服务运维助手",
		Instruction:   BuildPrompt(cfg.MemoryManifest),
		Model:         cm,
		MaxIterations: 10,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{Tools: decoratedTools(reg)},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("构建 ChatModelAgent 失败: %w", err)
	}
	return &einoEngine{
		runner: adk.NewRunner(context.Background(), adk.RunnerConfig{
			Agent:           agent,
			EnableStreaming: true, // 真实模型走 Stream,SSE 增量输出
		}),
		reg:       reg,
		modelName: cfg.Model,
	}, nil
}

func decoratedTools(reg *tools.Registry) []tool.BaseTool {
	names := reg.Names()
	out := make([]tool.BaseTool, 0, len(names))
	for _, name := range names {
		out = append(out, DecorateTool(name, reg.ByName(name)))
	}
	return out
}

// ID 后端标识。
func (e *einoEngine) ID() string { return "internal" }

// Label 展示名。
func (e *einoEngine) Label() string { return "内置助手 (eino)" }

// ModelName 返回模型名(状态接口展示)。
func (e *einoEngine) ModelName() string { return e.modelName }

// Close 无需释放资源。
func (e *einoEngine) Close() error { return nil }

// SetPolicy 注入工具权限策略。
func (e *einoEngine) SetPolicy(p *tools.Policy) {
	if e.reg != nil {
		e.reg.SetPolicy(p)
	}
}

// SetDelegator 注入外部 coding agent 委派执行器(可选)。
func (e *einoEngine) SetDelegator(d delegate.Runner) {
	if e.reg != nil {
		e.reg.SetDelegator(d)
	}
}

// DelegateStatus 查委派任务状态(外部 agent 委派是异步的)。
func (e *einoEngine) DelegateStatus(taskID string) (map[string]any, bool) {
	if e.reg == nil {
		return nil, false
	}
	return e.reg.DelegateStatus(taskID)
}

// SetMemory 注入记忆库与 prompt 摘要(可选)。
func (e *einoEngine) SetMemory(mem tools.MemStore, manifest string) {
	if e.reg != nil {
		e.reg.SetMemoryStore(mem)
	}
	e.memManifest = manifest
}

// Run 执行一轮对话:读会话历史 → 跑 agent → 事件推入返回的 chan。
func (e *einoEngine) Run(ctx context.Context, sess engine.Session, in engine.Input) (<-chan engine.Event, context.CancelFunc, error) {
	// 独立可取消:消费方断开时 cancel,模型流与事件发送随之终止
	ctx, cancel := context.WithCancel(ctx)

	out := make(chan engine.Event, 64)
	// emit 是唯一的事件出口:select 双路保证消费方消失后 goroutine 必然收敛,
	// 不会因 chan 无消费者而永久阻塞(否则信号量永不释放,AI 功能自锁)。
	emit := func(ev engine.Event) {
		select {
		case out <- ev:
		case <-ctx.Done():
		}
	}
	ctx = withEventSink(ctx, func(ev engine.Event) { emit(ev) })
	// 工具层要拿到会话能力(危险工具确认、权限档)。
	ctx = tools.WithScope(ctx, sess)
	// 角色由入口放进 ctx(HTTP 的 AI chat 经鉴权中间件设角色)。
	// **这里不做「无角色当 admin」的兜底** —— 那会让「某个入口忘了设角色」
	// 变成「以管理员身份运行」。缺角色就只能是只读(安全侧)。

	msgs := historyMessages(sess.History())

	async.GoSafe(func() {
		defer close(out)
		defer cancel()
		// 自带 recover:panic 要作为 error 事件推给客户端,不能只靠 GoSafe 的全局日志。
		defer func() {
			if rec := recover(); rec != nil {
				emit(engine.Event{Type: engine.EventError, Content: fmt.Sprintf("内部错误: %v", rec)})
				slog.Error("ai agent panic", "panic", rec, "session", sess.SessionID())
			}
		}()

		iter := e.runner.Run(ctx, msgs)
		var (
			full  strings.Builder
			usage engine.Usage
		)
		for {
			ev, ok := iter.Next()
			if !ok {
				break
			}
			if ev.Err != nil {
				emit(engine.Event{Type: engine.EventError, Content: errText(ev.Err)})
				sess.Append("assistant", full.String())
				return
			}
			if ev.Output == nil || ev.Output.MessageOutput == nil {
				continue
			}
			mo := ev.Output.MessageOutput
			if mo.Role == schema.Tool {
				continue // 工具结果已由装饰器上报
			}
			if mo.IsStreaming && mo.MessageStream != nil {
				for {
					chunk, err := mo.MessageStream.Recv()
					if errors.Is(err, io.EOF) {
						break
					}
					if err != nil {
						mo.MessageStream.Close()
						emit(engine.Event{Type: engine.EventError, Content: errText(err)})
						sess.Append("assistant", full.String())
						return
					}
					if chunk == nil {
						continue
					}
					if len(chunk.ToolCalls) > 0 {
						continue // 工具决策由装饰器上报
					}
					if chunk.Content != "" {
						full.WriteString(chunk.Content)
						emit(engine.Event{Type: engine.EventDelta, Content: chunk.Content})
					}
					accumulateUsage(&usage, chunk)
				}
			} else if mo.Message != nil {
				if mo.Message.Content != "" {
					full.WriteString(mo.Message.Content)
					emit(engine.Event{Type: engine.EventDelta, Content: mo.Message.Content})
				}
				accumulateUsage(&usage, mo.Message)
			}
		}
		sess.Append("assistant", full.String())
		emit(engine.Event{Type: engine.EventDone, Usage: &usage})
	})
	return out, cancel, nil
}

// ExecuteConfirmed 确认后直达执行:校验令牌 → 标记放行 → 直调工具 →
// 结果写入会话(不经过模型)。返回工具输出的 JSON 文本。
func (e *einoEngine) ExecuteConfirmed(ctx context.Context, sess engine.Session, toolName, token string) (string, error) {
	tl := e.reg.ByName(toolName)
	if tl == nil {
		return "", fmt.Errorf("未知工具 %q", toolName)
	}
	args, err := sess.ConsumePending(toolName, token)
	if err != nil {
		return "", err
	}
	sess.MarkConsumed(toolName, args)

	out, err := tl.InvokableRun(tools.WithScope(ctx, sess), args)
	if err != nil {
		out = failJSON(err)
	}
	sess.Append("assistant", fmt.Sprintf("(已执行 %s)%s", toolName, out))
	return out, nil
}

func historyMessages(turns []engine.Turn) []*schema.Message {
	out := make([]*schema.Message, 0, len(turns))
	for _, m := range turns {
		role := schema.User
		switch m.Role {
		case "assistant":
			role = schema.Assistant
		case "system":
			role = schema.System
		}
		out = append(out, &schema.Message{Role: role, Content: m.Content})
	}
	return out
}

func accumulateUsage(u *engine.Usage, msg *schema.Message) {
	if msg == nil || msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
		return
	}
	u.InputTokens += msg.ResponseMeta.Usage.PromptTokens
	u.OutputTokens += msg.ResponseMeta.Usage.CompletionTokens
}

func errText(err error) string {
	if errors.Is(err, context.Canceled) {
		return "已取消"
	}
	return fmt.Sprintf("模型调用失败: %v", err)
}

// failJSON 把确认执行失败编码为 JSON 对象。
func failJSON(err error) string {
	b, _ := json.Marshal(map[string]any{"status": "failed", "error": err.Error()})
	return string(b)
}
