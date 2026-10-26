package agent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/agent/engine/delegate"
	"github.com/yi-nology/git-ferry/internal/agent/tools"
	"github.com/yi-nology/git-ferry/internal/agent/tools/toolstest"
	"github.com/yi-nology/git-ferry/internal/pkg/authz"
)

// fakeDelegator 记录被委派的任务(委派链路测试用)。
type fakeDelegator struct {
	calledCh chan struct{}
	got      delegate.Request
	out      string
}

func (f *fakeDelegator) Delegate(_ context.Context, req delegate.Request) (string, error) {
	f.got = req
	select {
	case f.calledCh <- struct{}{}:
	default:
	}
	return f.out, nil
}

// Test委派链路端到端 内置助手(eino + 假模型)→ **真实的** delegate 工具 →
// 委派执行器。走完「模型发起 → 危险工具要确认 → 确认后真的派出去」全链路。
func Test委派链路端到端(t *testing.T) {
	d := &fakeDelegator{calledCh: make(chan struct{}, 1), out: "外部 agent 干完了:改了 2 个文件"}
	fm := toolstest.NewFakeModel()
	fm.Append(toolstest.FakeStep{ToolName: "ai_code_delegate", ToolArgs: `{"prompt":"修个登录 bug"}`})
	fm.Append(toolstest.FakeStep{Content: "已委托外部 agent"})

	reg := tools.NewRegistry(toolstest.NewMock())
	reg.SetDelegator(d)
	eng, err := NewEinoEngineWithModel(&Config{}, fm, reg)
	require.NoError(t, err)
	r := NewRunner(eng, 2)
	sess := r.Sessions().Create()

	ctx := authz.WithRole(context.Background(), authz.RoleAdmin)
	ch, cancel, err := r.Run(ctx, sess, "帮我改个登录 bug")
	require.NoError(t, err)
	defer cancel()

	// 第一轮:delegate 是危险工具 ⇒ 必须先要确认,不能直接派出去
	events := drainAll(t, ch)
	var confirm *Event
	for i := range events {
		if events[i].Type == "tool_confirm" && events[i].Tool == "ai_code_delegate" {
			confirm = &events[i]
		}
	}
	require.NotNil(t, confirm, "委派外部 agent 必须走确认链路")
	select {
	case <-d.calledCh:
		t.Fatal("未确认前不得真的派出去")
	default:
	}

	// 确认直达执行:**异步派发** ⇒ 立刻返回任务 ID,不等 agent 跑完
	out, err := r.ExecuteConfirmed(ctx, sess, "ai_code_delegate", confirm.Token)
	require.NoError(t, err)
	assert.Contains(t, out, `"status":"dispatched"`)
	assert.Contains(t, out, `"task_id":"dtask-`)

	// 等委派真正被发起
	select {
	case <-d.calledCh:
	case <-time.After(5 * time.Second):
		t.Fatal("等待委派发起超时")
	}
	assert.Equal(t, "修个登录 bug", d.got.Prompt, "prompt 要原样传给委派执行器")
}

// Test委派链路_参数覆盖 允许单次委派覆盖 agent 名与工作目录。
func Test委派链路_参数覆盖(t *testing.T) {
	d := &fakeDelegator{calledCh: make(chan struct{}, 1), out: "ok"}
	fm := toolstest.NewFakeModel()
	fm.Append(toolstest.FakeStep{ToolName: "ai_code_delegate",
		ToolArgs: `{"prompt":"p","agent":"codex","cwd":"/tmp/ws"}`})
	reg := tools.NewRegistry(toolstest.NewMock())
	reg.SetDelegator(d)
	eng, err := NewEinoEngineWithModel(&Config{}, fm, reg)
	require.NoError(t, err)
	r := NewRunner(eng, 2)
	sess := r.Sessions().Create()

	ctx := authz.WithRole(context.Background(), authz.RoleAdmin)
	ch, cancel, err := r.Run(ctx, sess, "干活")
	require.NoError(t, err)
	defer cancel()
	events := drainAll(t, ch)
	var confirm *Event
	for i := range events {
		if events[i].Type == "tool_confirm" {
			confirm = &events[i]
		}
	}
	require.NotNil(t, confirm)
	_, err = r.ExecuteConfirmed(ctx, sess, "ai_code_delegate", confirm.Token)
	require.NoError(t, err)
	// 异步派发 ⇒ 等它真的被发起再断言参数
	select {
	case <-d.calledCh:
	case <-time.After(5 * time.Second):
		t.Fatal("等待委派发起超时")
	}
	assert.Equal(t, "codex", d.got.Agent, "agent 覆盖要传下去")
	assert.Equal(t, "/tmp/ws", d.got.Cwd, "工作目录覆盖要传下去")
}

// Test委派链路_未装配执行器时如实报错 未装配时工具应返回说明,而不是静默成功。
func Test委派链路_未装配执行器时如实报错(t *testing.T) {
	fm := toolstest.NewFakeModel()
	fm.Append(toolstest.FakeStep{ToolName: "ai_code_delegate", ToolArgs: `{"prompt":"x"}`})
	reg := tools.NewRegistry(toolstest.NewMock())
	// **不** SetDelegator
	eng, err := NewEinoEngineWithModel(&Config{}, fm, reg)
	require.NoError(t, err)
	r := NewRunner(eng, 2)
	sess := r.Sessions().Create()

	ctx := authz.WithRole(context.Background(), authz.RoleAdmin)
	ch, cancel, err := r.Run(ctx, sess, "干活")
	require.NoError(t, err)
	defer cancel()
	events := drainAll(t, ch)
	var confirm *Event
	for i := range events {
		if events[i].Type == "tool_confirm" {
			confirm = &events[i]
		}
	}
	require.NotNil(t, confirm)
	out, err := r.ExecuteConfirmed(ctx, sess, "ai_code_delegate", confirm.Token)
	require.NoError(t, err)
	assert.Contains(t, out, "未配置", "未装配执行器时要如实说明")
}

// drainAll 收完一轮事件。
func drainAll(t *testing.T, ch <-chan Event) []Event {
	t.Helper()
	var events []Event
	deadline := time.After(20 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return events
			}
			events = append(events, ev)
		case <-deadline:
			t.Fatal("等待事件超时")
		}
	}
}
