package agent

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/yi-nology/git-ferry/internal/agent/tools"
)

type sinkKey struct{}

type sinkFunc func(Event)

func withEventSink(ctx context.Context, sink sinkFunc) context.Context {
	return context.WithValue(ctx, sinkKey{}, sink)
}

func sinkFromCtx(ctx context.Context) sinkFunc {
	sink, _ := ctx.Value(sinkKey{}).(sinkFunc)
	if sink == nil {
		return func(Event) {} // 缺省丢弃,装饰器永不为 nil 调用
	}
	return sink
}

// DecorateTool 包装工具,经 ctx 中的 sink 产出 tool_start/tool_end/tool_confirm
// 事件。危险工具返回确认标记时升级为 tool_confirm(携带令牌),前端据此弹确认卡。
func DecorateTool(name string, inner tool.InvokableTool) tool.InvokableTool {
	return &decoratedTool{name: name, inner: inner}
}

type decoratedTool struct {
	name  string
	inner tool.InvokableTool
}

func (d *decoratedTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return d.inner.Info(ctx)
}

func (d *decoratedTool) InvokableRun(ctx context.Context, argsJSON string, opts ...tool.Option) (string, error) {
	sink := sinkFromCtx(ctx)
	sink(Event{Type: "tool_start", Tool: d.name, Args: truncate(argsJSON, 300)})

	out, err := d.inner.InvokableRun(ctx, argsJSON, opts...)
	if err != nil {
		// 工具失败转 JSON 给模型,不中断 agent,由模型向用户解释
		out = toolErrorJSON(err)
		sink(Event{Type: "tool_end", Tool: d.name, Result: truncate(out, resultCap)})
		return out, nil
	}
	// 超大输出落盘,只回预览(控制上下文成本)
	out = persistResult(d.name, out)
	if tools.DangerTools[d.name] && strings.Contains(out, `"status":"confirmation_required"`) {
		ev := Event{Type: "tool_confirm", Tool: d.name, Args: truncate(argsJSON, 300)}
		if sc := tools.ScopeFrom(ctx); sc != nil {
			if name, token, args := sc.LastConfirm(); name == d.name {
				ev.Token, ev.Args = token, truncate(args, 300)
			}
		}
		sink(ev)
		return out, nil
	}
	sink(Event{Type: "tool_end", Tool: d.name, Result: truncate(out, resultCap)})
	return out, nil
}

// toolErrorJSON 把工具错误编码为 JSON 对象(不手拼字符串,防转义问题)。
func toolErrorJSON(err error) string {
	b, _ := json.Marshal(map[string]any{"error": err.Error()})
	return string(b)
}
