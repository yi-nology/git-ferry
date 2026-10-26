package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/agent/engine/delegate"
	"github.com/yi-nology/git-ferry/internal/agent/tools/toolstest"
	"github.com/yi-nology/git-ferry/internal/pkg/authz"
)

// fakeDelegator 记录被委派的任务。
type fakeDelegator struct {
	called bool
	got    delegate.Request
	out    string
	err    error
}

func (f *fakeDelegator) Delegate(_ context.Context, req delegate.Request) (string, error) {
	f.called = true
	f.got = req
	return f.out, f.err
}

// TestDelegate_RequiresConfirmation 委派外部 agent 是危险操作:
// 外部 agent 在**服务端主机上**跑 ⇒ 必须走确认链路。
func TestDelegate_RequiresConfirmation(t *testing.T) {
	reg := NewRegistry(toolstest.NewMock())
	d := &fakeDelegator{out: "done"}
	reg.SetDelegator(d)
	sc := &fakeScope{}
	ctx := WithScope(authz.WithRole(context.Background(), authz.RoleAdmin), sc)

	out, err := reg.ByName("ai_code_delegate").InvokableRun(ctx, `{"prompt":"改个 bug"}`)
	require.NoError(t, err)
	assert.Contains(t, out, `"status":"confirmation_required"`)
	assert.False(t, d.called, "未确认前不得真的把活派出去")
	assert.Equal(t, "ai_code_delegate", sc.lastTool)
}

// TestDelegate_ExecutesAfterConfirmation 确认后确实委派出去。
func TestDelegate_ExecutesAfterConfirmation(t *testing.T) {
	reg := NewRegistry(toolstest.NewMock())
	d := &fakeDelegator{out: "已修好"}
	reg.SetDelegator(d)
	sc := &fakeScope{}
	ctx := WithScope(authz.WithRole(context.Background(), authz.RoleAdmin), sc)

	args := `{"prompt":"改个 bug"}`
	_, err := reg.ByName("ai_code_delegate").InvokableRun(ctx, args)
	require.NoError(t, err)

	sc.MarkConsumed("ai_code_delegate", args)
	out, err := reg.ByName("ai_code_delegate").InvokableRun(ctx, args)
	require.NoError(t, err)
	assert.Contains(t, out, "已修好")
	assert.True(t, d.called)
	assert.Equal(t, "改个 bug", d.got.Prompt)
}

// TestDelegate_NotConfigured 未装配委派执行器时如实报错,不静默失败。
func TestDelegate_NotConfigured(t *testing.T) {
	reg := NewRegistry(toolstest.NewMock())
	sc := &fakeScope{}
	ctx := WithScope(authz.WithRole(context.Background(), authz.RoleAdmin), sc)

	args := `{"prompt":"改个 bug"}`
	_, err := reg.ByName("ai_code_delegate").InvokableRun(ctx, args)
	require.NoError(t, err)
	sc.MarkConsumed("ai_code_delegate", args)
	out, err := reg.ByName("ai_code_delegate").InvokableRun(ctx, args)
	require.NoError(t, err)
	assert.Contains(t, out, "未配置")
}

// TestDelegate_RefusedForReadonlyRole 只读角色不能把活派到外部 agent。
func TestDelegate_RefusedForReadonlyRole(t *testing.T) {
	reg := NewRegistry(toolstest.NewMock())
	d := &fakeDelegator{out: "done"}
	reg.SetDelegator(d)
	ctx := WithScope(authz.WithRole(context.Background(), authz.RoleReadonly), &fakeScope{})

	out, err := reg.ByName("ai_code_delegate").InvokableRun(ctx, `{"prompt":"改个 bug"}`)
	require.NoError(t, err)
	assert.Contains(t, out, "forbidden")
	assert.False(t, d.called)
}

// TestDelegate_IsDangerTool 危险标注必须在注册处(全仓唯一事实源)。
func TestDelegate_IsDangerTool(t *testing.T) {
	reg := NewRegistry(toolstest.NewMock())
	assert.True(t, reg.IsDanger("ai_code_delegate"),
		"委派外部 agent 必须在注册处标危险")
}
