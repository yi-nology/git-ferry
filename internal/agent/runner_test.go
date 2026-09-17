package agent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/agent/tools"
	"github.com/yi-nology/git-ferry/internal/agent/tools/toolstest"
)

func newTestRunner(t *testing.T, m *toolstest.Mock, steps ...toolstest.FakeStep) *Runner {
	t.Helper()
	fm := toolstest.NewFakeModel()
	for _, s := range steps {
		fm.Append(s)
	}
	r, err := NewRunnerWithModel(&Config{MaxConcurrentChats: 2}, fm, tools.NewRegistry(m))
	require.NoError(t, err)
	return r
}

// drainEvents 收完事件流,返回累计文本与事件列表。
func drainEvents(t *testing.T, ch <-chan Event) (string, []Event) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	var (
		text   string
		events []Event
	)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return text, events
			}
			events = append(events, ev)
			if ev.Type == "delta" {
				text += ev.Content
			}
			if ev.Type == "error" {
				t.Fatalf("runner 出错: %s", ev.Content)
			}
		case <-deadline:
			t.Fatal("等待事件超时")
		}
	}
}

func TestRunner_TextReply(t *testing.T) {
	r := newTestRunner(t, toolstest.NewMock(), toolstest.FakeStep{Content: "你好,我是同步助手"})
	st := NewSessionStore(time.Minute, 20)
	sess := st.Create()

	ch, err := r.Run(context.Background(), sess, "你是谁")
	require.NoError(t, err)
	text, events := drainEvents(t, ch)

	assert.Equal(t, "你好,我是同步助手", text)
	assert.Equal(t, "start", events[0].Type)
	assert.Equal(t, sess.ID, events[0].SessionID)
	assert.Equal(t, "done", events[len(events)-1].Type)
	assert.Len(t, sess.Messages, 2, "会话应存 user+assistant 两条")
	assert.Equal(t, "你好,我是同步助手", sess.Messages[1].Content)
}

func TestRunner_ToolLoop_WithConfirm(t *testing.T) {
	m := toolstest.NewMock()
	r := newTestRunner(t, m,
		toolstest.FakeStep{ToolName: "run_task", ToolArgs: `{"task_key":"t1"}`},
		toolstest.FakeStep{Content: "已发送确认请求,请在确认后执行"},
	)
	st := NewSessionStore(time.Minute, 20)
	sess := st.Create()

	ch, err := r.Run(context.Background(), sess, "帮我同步 t1")
	require.NoError(t, err)
	_, events := drainEvents(t, ch)

	var confirm *Event
	for i := range events {
		if events[i].Type == "tool_confirm" {
			confirm = &events[i]
		}
	}
	require.NotNil(t, confirm, "应有 tool_confirm 事件")
	assert.Equal(t, "run_task", confirm.Tool)
	assert.NotEmpty(t, confirm.Token)
	assert.Empty(t, m.RunTaskKey(), "未确认不执行")

	// 确认直达执行(不经模型)
	out, err := r.ExecuteConfirmed(context.Background(), sess, "run_task", confirm.Token)
	require.NoError(t, err)
	assert.Contains(t, out, `"status":"ok"`)
	assert.Equal(t, "t1", m.RunTaskKey())

	// 令牌一次性:再次执行报错
	_, err = r.ExecuteConfirmed(context.Background(), sess, "run_task", confirm.Token)
	assert.Error(t, err)
}

func TestRunner_Busy(t *testing.T) {
	fm := toolstest.NewFakeModel()
	hold := make(chan struct{})
	fm.Hold = hold
	fm.Append(toolstest.FakeStep{Content: "long"})
	r, err := NewRunnerWithModel(&Config{MaxConcurrentChats: 1}, fm, tools.NewRegistry(toolstest.NewMock()))
	require.NoError(t, err)
	st := NewSessionStore(time.Minute, 20)

	ch1, err := r.Run(context.Background(), st.Create(), "a")
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond) // 等 goroutine 进入模型调用并占住信号量

	_, err = r.Run(context.Background(), st.Create(), "b")
	assert.ErrorIs(t, err, ErrBusy)

	close(hold) // 放行
	drainEvents(t, ch1)
}

func TestRunner_SessionStoreAttached(t *testing.T) {
	r := newTestRunner(t, toolstest.NewMock(), toolstest.FakeStep{Content: "ok"})
	st := r.Sessions()
	require.NotNil(t, st)
	sess := st.Create()
	// Session 适配 tools.SessionScope:经 Session 代理到 store
	_, err := sess.SetPending("run_task", `{}`)
	require.NoError(t, err)
	assert.NotNil(t, sess.PendingSnapshot())
}
