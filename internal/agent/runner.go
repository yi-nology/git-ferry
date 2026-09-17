package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/cloudwego/eino-ext/components/model/openai"

	"github.com/yi-nology/git-ferry/internal/agent/tools"
)

// Runner AI 助手编排器:eino ChatModelAgent + 工具装饰器 + 会话存储。
type Runner struct {
	runner   *adk.Runner
	reg      *tools.Registry
	sessions *SessionStore
	sem      chan struct{}
	modelName string
}

// NewRunner 生产构造:OpenAI 兼容模型(BaseURL 可指内网 vLLM/Ollama)。
func NewRunner(cfg *Config, apiKey string, svc tools.SyncService) (*Runner, error) {
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
	return NewRunnerWithModel(cfg, cm, tools.NewRegistry(svc))
}

// NewRunnerWithModel 以注入模型构造(测试用)。
func NewRunnerWithModel(cfg *Config, cm model.BaseModel[*schema.Message], reg *tools.Registry) (*Runner, error) {
	agent, err := adk.NewChatModelAgent(context.Background(), &adk.ChatModelAgentConfig{
		Name:          "git-sync-assistant",
		Description:   "仓库同步服务运维助手",
		Instruction:   Prompt,
		Model:         cm,
		MaxIterations: 10,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{Tools: decoratedTools(reg)},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("构建 ChatModelAgent 失败: %w", err)
	}
	maxChats := cfg.MaxConcurrentChats
	if maxChats <= 0 {
		maxChats = 4
	}
	return &Runner{
		runner: adk.NewRunner(context.Background(), adk.RunnerConfig{
			Agent:           agent,
			EnableStreaming: true, // 真实模型走 Stream,SSE 增量输出
		}),
		reg:       reg,
		sessions:  NewSessionStore(sessionTTL, sessionMaxRounds),
		sem:       make(chan struct{}, maxChats),
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

// Sessions 暴露会话存储(handler 建会话用)。
func (r *Runner) Sessions() *SessionStore { return r.sessions }

// ModelName 返回模型名(状态接口展示)。
func (r *Runner) ModelName() string { return r.modelName }

// Run 执行一轮对话:写入用户消息 → 跑 agent → 事件推入返回的 chan。
// chan 关闭即本轮结束;error 事件后 chan 也会关闭。
func (r *Runner) Run(ctx context.Context, sess *Session, userMsg string) (<-chan Event, error) {
	select {
	case r.sem <- struct{}{}:
	default:
		return nil, ErrBusy
	}

	out := make(chan Event, 64)
	sink := func(e Event) {
		select {
		case out <- e:
		case <-ctx.Done():
		}
	}
	ctx = withEventSink(ctx, sink)
	ctx = tools.WithScope(ctx, sess)

	r.sessions.Append(sess, "user", userMsg)
	msgs := historyMessages(sess.Messages)

	go func() {
		defer close(out)
		defer func() { <-r.sem }()
		defer func() {
			if rec := recover(); rec != nil {
				out <- Event{Type: "error", Content: fmt.Sprintf("内部错误: %v", rec)}
				slog.Error("ai agent panic", "panic", rec, "session", sess.ID)
			}
		}()

		out <- Event{Type: "start", SessionID: sess.ID}

		iter := r.runner.Run(ctx, msgs)
		var (
			full  strings.Builder
			usage Usage
		)
		for {
			ev, ok := iter.Next()
			if !ok {
				break
			}
			if ev.Err != nil {
				out <- Event{Type: "error", Content: errText(ev.Err)}
				r.sessions.Append(sess, "assistant", full.String())
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
						out <- Event{Type: "error", Content: errText(err)}
						r.sessions.Append(sess, "assistant", full.String())
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
						out <- Event{Type: "delta", Content: chunk.Content}
					}
					accumulateUsage(&usage, chunk)
				}
			} else if mo.Message != nil {
				if mo.Message.Content != "" {
					full.WriteString(mo.Message.Content)
					out <- Event{Type: "delta", Content: mo.Message.Content}
				}
				accumulateUsage(&usage, mo.Message)
			}
		}
		r.sessions.Append(sess, "assistant", full.String())
		out <- Event{Type: "done", Usage: &usage}
	}()
	return out, nil
}

// ExecuteConfirmed 确认后直达执行:校验令牌 → 标记放行 → 直调工具 →
// 结果写入会话(不经过模型)。返回工具输出的 JSON 文本。
func (r *Runner) ExecuteConfirmed(ctx context.Context, sess *Session, toolName, token string) (string, error) {
	tl := r.reg.ByName(toolName)
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
		out = fmt.Sprintf(`{"status":"failed","error":%q}`, err.Error())
	}
	r.sessions.Append(sess, "assistant", fmt.Sprintf("(已执行 %s)%s", toolName, out))
	return out, nil
}

func historyMessages(msgs []Message) []*schema.Message {
	out := make([]*schema.Message, 0, len(msgs))
	for _, m := range msgs {
		role := schema.User
		if m.Role == "assistant" {
			role = schema.Assistant
		}
		out = append(out, &schema.Message{Role: role, Content: m.Content})
	}
	return out
}

func accumulateUsage(u *Usage, msg *schema.Message) {
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
