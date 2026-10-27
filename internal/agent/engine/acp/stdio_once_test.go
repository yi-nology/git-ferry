package acp

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/agent/engine/delegate"
)

// delegateRequest 造一个委派请求。
func delegateRequest(prompt string) delegate.Request {
	return delegate.Request{Prompt: prompt}
}

// delegateRequestWithAgent 造一个指定 agent 名的委派请求。
func delegateRequestWithAgent(agent, prompt string) delegate.Request {
	return delegate.Request{Prompt: prompt, Agent: agent}
}

// TestRunOnceStdio_假agent跑通一次 stdio 形态的一次性执行:
// initialize → session/new → session/prompt → 收结果 → 关进程。
//
// **为什么要有这条**:具名 profile 里可以是 stdio 形态的 agent(如 "mcode acp"),
// 委派按名字选到它时必须能真跑 —— 硬塞给 acpx(--agent "<command>")
// 是把命令当成 acpx 的 agent 名,必然跑不起来。
func TestRunOnceStdio_假agent跑通一次(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	text, err := RunOnceStdio(ctx, &Config{
		Command:    fakeACPCommand(t),
		Cwd:        t.TempDir(),
		Timeout:    30 * time.Second,
		Permission: PermissionApproveReads,
	}, "写个 hello")
	require.NoError(t, err)
	assert.Contains(t, text, "你好，我是假 agent")
}

// TestRunOnceStdio_ask档要拒 一次性执行弹不出授权卡 ⇒ ask 没有对应物。
// **不静默降级**:直接报错,而不是偷偷按 approve_reads 跑。
func TestRunOnceStdio_ask档要拒(t *testing.T) {
	_, err := RunOnceStdio(context.Background(), &Config{
		Command:    fakeACPCommand(t),
		Cwd:        t.TempDir(),
		Timeout:    30 * time.Second,
		Permission: PermissionAsk,
	}, "hi")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "非交互")
}

// TestDelegateRunner_stdio传输走stdio 委派到 stdio profile 时,不能把它
// 当成 acpx 的 agent 名 —— 那是「命令」不是「名字」。
func TestDelegateRunner_stdio传输走stdio(t *testing.T) {
	dr, err := NewDelegateRunner(&Config{
		Transport:  TransportStdio,
		Command:    fakeACPCommand(t),
		Cwd:        t.TempDir(),
		Timeout:    30 * time.Second,
		Permission: PermissionApproveReads,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	text, err := dr.Delegate(ctx, delegateRequest("写个 hello"))
	require.NoError(t, err)
	assert.Contains(t, text, "你好，我是假 agent")
}

// TestDelegateRunner_stdio下用名字选不存在的profile要报错 stdio 没有
// 「内置 agent 名」这回事 ⇒ 拿名字当命令跑必然失败,而且看不出真实原因。
func TestDelegateRunner_stdio下用名字选不存在的profile要报错(t *testing.T) {
	dr, err := NewDelegateRunner(&Config{
		Transport:  TransportStdio,
		Command:    fakeACPCommand(t),
		Cwd:        t.TempDir(),
		Timeout:    30 * time.Second,
		Permission: PermissionApproveReads,
	})
	require.NoError(t, err)

	_, err = dr.Delegate(context.Background(), delegateRequestWithAgent("codex", "写个 hello"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "codex")
}
