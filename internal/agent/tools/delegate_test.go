package tools

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/agent/engine/delegate"
	"github.com/yi-nology/git-ferry/internal/agent/tools/toolstest"
	"github.com/yi-nology/git-ferry/internal/pkg/authz"
)

// fakeDelegator 记录被委派的任务(委派链路测试用)。
type fakeDelegator struct {
	calledCh chan struct{}
	got      delegate.Request
	out      string
	err      error
}

func newFakeDelegator(out string) *fakeDelegator {
	return &fakeDelegator{calledCh: make(chan struct{}, 1), out: out}
}

func (f *fakeDelegator) Delegate(_ context.Context, req delegate.Request) (string, error) {
	f.got = req
	select {
	case f.calledCh <- struct{}{}:
	default:
	}
	return f.out, f.err
}

// waitCalled 等委派被真正发起(异步派发 ⇒ 不能同步断言)。
func (f *fakeDelegator) waitCalled(t *testing.T) {
	t.Helper()
	select {
	case <-f.calledCh:
	case <-time.After(5 * time.Second):
		t.Fatal("等待委派发起超时")
	}
}

// TestDelegate_RequiresConfirmation 委派外部 agent 是危险操作:
// 外部 agent 是本机子进程 ⇒ 必须先要确认。
func TestDelegate_RequiresConfirmation(t *testing.T) {
	reg := NewRegistry(toolstest.NewMock())
	d := newFakeDelegator("已修好")
	reg.SetDelegator(d)
	sc := &fakeScope{}
	ctx := WithScope(authz.WithRole(context.Background(), authz.RoleAdmin), sc)

	out, err := reg.ByName("ai_code_delegate").InvokableRun(ctx, `{"prompt":"改个 bug"}`)
	require.NoError(t, err)
	assert.Contains(t, out, `"status":"confirmation_required"`)
	select {
	case <-d.calledCh:
		t.Fatal("未确认前不得真的派出去")
	default:
	}
}

// TestDelegate_ExecutesAfterConfirmation 确认后**异步派发**:
// 立刻返回任务 ID(不等 agent 跑完 —— 同步等会把整条 SSE 挂住,
// 中间的代理超时就会切断连接)。
func TestDelegate_ExecutesAfterConfirmation(t *testing.T) {
	reg := NewRegistry(toolstest.NewMock())
	d := newFakeDelegator("已修好")
	reg.SetDelegator(d)
	sc := &fakeScope{}
	ctx := WithScope(authz.WithRole(context.Background(), authz.RoleAdmin), sc)

	args := `{"prompt":"改个 bug"}`
	_, err := reg.ByName("ai_code_delegate").InvokableRun(ctx, args)
	require.NoError(t, err)

	sc.MarkConsumed("ai_code_delegate", args)
	out, err := reg.ByName("ai_code_delegate").InvokableRun(ctx, args)
	require.NoError(t, err)
	assert.Contains(t, out, `"status":"dispatched"`)
	assert.Contains(t, out, `"task_id":"dtask-`)

	d.waitCalled(t)
	assert.Equal(t, "改个 bug", d.got.Prompt, "prompt 要原样传给委派执行器")
}

// TestDelegate_结果可按任务ID取回 派发后结果落到任务表里,
// 助手可按 task_id 取回(不必等用户再问一轮)。
func TestDelegate_结果可按任务ID取回(t *testing.T) {
	reg := NewRegistry(toolstest.NewMock())
	d := newFakeDelegator("外部 agent 的结论")
	reg.SetDelegator(d)
	sc := &fakeScope{}
	ctx := WithScope(authz.WithRole(context.Background(), authz.RoleAdmin), sc)

	args := `{"prompt":"干活"}`
	_, err := reg.ByName("ai_code_delegate").InvokableRun(ctx, args)
	require.NoError(t, err)
	sc.MarkConsumed("ai_code_delegate", args)
	out, err := reg.ByName("ai_code_delegate").InvokableRun(ctx, args)
	require.NoError(t, err)

	// 从回执里取 task_id
	var receipt struct {
		TaskID string `json:"task_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &receipt))
	require.NotEmpty(t, receipt.TaskID)

	d.waitCalled(t)
	// 等任务标记完成
	deadline := time.Now().Add(5 * time.Second)
	var got string
	for time.Now().Before(deadline) {
		got, err = reg.ByName("ai_delegate_result").InvokableRun(ctx, `{"task_id":"`+receipt.TaskID+`"}`)
		require.NoError(t, err)
		if !strings.Contains(got, `"status":"running"`) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	assert.Contains(t, got, "外部 agent 的结论")
	assert.Contains(t, got, `"status":"done"`)
}

// TestDelegate_结果回填会话 任务完成后结果回到会话里 ⇒
// 用户下一轮问话时助手**已经**看得见结果,不必再专门问一次。
func TestDelegate_结果回填会话(t *testing.T) {
	reg := NewRegistry(toolstest.NewMock())
	d := newFakeDelegator("回填的结论")
	reg.SetDelegator(d)
	sc := &appendingScope{}
	ctx := WithScope(authz.WithRole(context.Background(), authz.RoleAdmin), sc)

	args := `{"prompt":"干活"}`
	_, err := reg.ByName("ai_code_delegate").InvokableRun(ctx, args)
	require.NoError(t, err)
	sc.MarkConsumed("ai_code_delegate", args)
	_, err = reg.ByName("ai_code_delegate").InvokableRun(ctx, args)
	require.NoError(t, err)

	d.waitCalled(t)
	deadline := time.Now().Add(5 * time.Second)
	var msgs []string
	for time.Now().Before(deadline) {
		msgs = sc.messages()
		if len(msgs) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	require.NotEmpty(t, msgs, "结果应回填到会话")
	assert.Contains(t, msgs[0], "回填的结论")
	assert.Contains(t, msgs[0], "仅作数据", "回填文本要标明是数据不是指令")
}

// TestDelegate_NotConfigured 未装配执行器时如实报错,不静默失败。
func TestDelegate_NotConfigured(t *testing.T) {
	reg := NewRegistry(toolstest.NewMock())
	sc := &fakeScope{}
	ctx := WithScope(authz.WithRole(context.Background(), authz.RoleAdmin), sc)

	args := `{"prompt":"x"}`
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
	d := newFakeDelegator("done")
	reg.SetDelegator(d)
	ctx := WithScope(authz.WithRole(context.Background(), authz.RoleReadonly), &fakeScope{})

	out, err := reg.ByName("ai_code_delegate").InvokableRun(ctx, `{"prompt":"x"}`)
	require.NoError(t, err)
	assert.Contains(t, out, "forbidden")
	select {
	case <-d.calledCh:
		t.Fatal("只读角色不该能派发")
	default:
	}
}

// TestDelegate_IsDangerTool 危险标注必须在注册处(全仓唯一事实源)。
func TestDelegate_IsDangerTool(t *testing.T) {
	reg := NewRegistry(toolstest.NewMock())
	assert.True(t, reg.IsDanger("ai_code_delegate"),
		"委派外部 agent 必须在注册处标危险")
	// 取结果只是查表 ⇒ 不标危险
	assert.False(t, reg.IsDanger("ai_delegate_result"))
}

// appendingScope 记录被追加的会话消息。
//
// **要加锁**:追加发生在后台协程(委派完成时),测试协程同时在读 ⇒ 不加锁就是 data race。
type appendingScope struct {
	fakeScope
	mu       sync.Mutex
	appended []string
}

func (s *appendingScope) Append(role, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appended = append(s.appended, content)
}

// messages 取快照(加锁读)。
func (s *appendingScope) messages() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.appended))
	copy(out, s.appended)
	return out
}
