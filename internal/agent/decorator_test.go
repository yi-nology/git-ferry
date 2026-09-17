package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/agent/tools"
)

type stubTool struct{ result string }

func (s *stubTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "stub"}, nil
}
func (s *stubTool) InvokableRun(context.Context, string, ...tool.Option) (string, error) {
	return s.result, nil
}

type errTool struct{ err error }

func (e *errTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "stub"}, nil
}
func (e *errTool) InvokableRun(context.Context, string, ...tool.Option) (string, error) {
	return "", e.err
}

type fakeScope struct {
	consumedTool string
	consumedArgs string
	lastTool     string
	lastToken    string
	lastArgs     string
}

func (f *fakeScope) SetPending(string, string) (string, error) { return "tok123", nil }
func (f *fakeScope) HasConsumed(toolName, argsJSON string) bool {
	return f.consumedTool == toolName && f.consumedArgs == argsJSON
}
func (f *fakeScope) MarkConsumed(toolName, argsJSON string) {
	f.consumedTool, f.consumedArgs = toolName, argsJSON
}
func (f *fakeScope) SetLastConfirm(toolName, token, argsJSON string) {
	f.lastTool, f.lastToken, f.lastArgs = toolName, token, argsJSON
}
func (f *fakeScope) LastConfirm() (string, string, string) {
	return f.lastTool, f.lastToken, f.lastArgs
}

func collectSink(store *[]Event) context.Context {
	return withEventSink(context.Background(), func(e Event) { *store = append(*store, e) })
}

func TestDecorator_ReadOnlyTool(t *testing.T) {
	var events []Event
	dt := DecorateTool("list_repos", &stubTool{result: `{"total":0}`})

	out, err := dt.InvokableRun(collectSink(&events), `{}`)
	require.NoError(t, err)
	assert.Contains(t, out, "total")

	require.Len(t, events, 2)
	assert.Equal(t, "tool_start", events[0].Type)
	assert.Equal(t, "list_repos", events[0].Tool)
	assert.Equal(t, "tool_end", events[1].Type)
	assert.Contains(t, events[1].Result, "total")
}

func TestDecorator_DangerTool_Confirm(t *testing.T) {
	var events []Event
	dt := DecorateTool("run_task", &stubTool{result: `{"status":"confirmation_required","tool":"run_task"}`})

	sc := &fakeScope{}
	sc.SetLastConfirm("run_task", "tok123", `{"task_key":"t1"}`) // dangerGuard 正常会先登记
	ctx := tools.WithScope(collectSink(&events), sc)
	out, err := dt.InvokableRun(ctx, `{"task_key":"t1"}`)
	require.NoError(t, err)
	assert.Contains(t, out, "confirmation_required")

	require.Len(t, events, 2)
	assert.Equal(t, "tool_confirm", events[1].Type)
	assert.NotEmpty(t, events[1].Token, "tool_confirm 应携带 LastConfirm 的令牌")
}

func TestDecorator_ToolError(t *testing.T) {
	var events []Event
	dt := DecorateTool("list_repos", &errTool{err: errors.New("db down")})

	out, err := dt.InvokableRun(collectSink(&events), `{}`)
	require.NoError(t, err) // 工具错误转 JSON 给模型,不中断 agent
	assert.Contains(t, out, "db down")
	require.Len(t, events, 2)
	assert.Equal(t, "tool_end", events[1].Type)
	assert.Contains(t, events[1].Result, "db down")
}

func TestDecorator_NoSink_NoPanic(t *testing.T) {
	dt := DecorateTool("list_repos", &stubTool{result: "ok"})
	out, err := dt.InvokableRun(context.Background(), `{}`)
	require.NoError(t, err)
	assert.Equal(t, "ok", out)
}
