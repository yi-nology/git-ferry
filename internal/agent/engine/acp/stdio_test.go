package acp

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/agent/engine"
)

// fakeACPCommand 造一个指向假 agent 的命令(绝对路径,免得受 cwd 影响)。
func fakeACPCommand(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("没有 python3,跳过假 agent 测试")
	}
	abs, err := filepath.Abs(filepath.Join("testdata", "fake_acp_agent.py"))
	require.NoError(t, err)
	return "python3 " + abs
}

// TestEngine_假agent跑通一轮 协议层端到端:initialize → session/new →
// session/prompt → session/update → done。不依赖模型与网络。
func TestEngine_假agent跑通一轮(t *testing.T) {
	eng, err := NewEngine(&Config{
		Command: fakeACPCommand(t),
		Cwd:     t.TempDir(),
		Timeout: 30 * time.Second,
	})
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	sess := newFakeSession("s1")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ch, engCancel, err := eng.Run(ctx, sess, engine.Input{Message: "hi"})
	require.NoError(t, err)
	defer engCancel()

	var (
		text   strings.Builder
		events []engine.Event
	)
	deadline := time.After(30 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				assert.Contains(t, text.String(), "你好，我是假 agent")
				assert.Equal(t, engine.EventDone, events[len(events)-1].Type)
				return
			}
			events = append(events, ev)
			if ev.Type == engine.EventDelta {
				text.WriteString(ev.Content)
			}
		case <-deadline:
			t.Fatal("等待事件超时")
		}
	}
}

// TestEngine_透出外部agent会话ID 外部 agent 与本服务可能共用同一个会话库
// (如 MiniMax Code 的 CLI 与桌面版)⇒ 必须把 agent 侧的会话 ID 透出来,
// 用户才能在那个界面上找到这次会话。
func TestEngine_透出外部agent会话ID(t *testing.T) {
	eng, err := NewEngine(&Config{
		Command: fakeACPCommand(t),
		Cwd:     t.TempDir(),
		Timeout: 30 * time.Second,
	})
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	sess := newFakeSession("s1")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ch, engCancel, err := eng.Run(ctx, sess, engine.Input{Message: "hi"})
	require.NoError(t, err)
	defer engCancel()

	var found *engine.Event
	deadline := time.After(30 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				require.NotNil(t, found, "应有 agent_session 事件")
				assert.Equal(t, "fake-acp-session-1", found.AgentSessionID)
				assert.Equal(t, "Fake ACP Agent", found.AgentLabel)
				return
			}
			if ev.Type == engine.EventAgentSession {
				e := ev
				found = &e
			}
		case <-deadline:
			t.Fatal("等待事件超时")
		}
	}
}

// TestConfigValidate_acpx不支持ask acpx 是**非交互**的(它的 stdin/stdout 被我们占着)
// ⇒ 弹不出授权卡;它的 nonInteractivePermissions 默认是 deny ⇒ 「问」实际等于「拒绝」。
// 配置与实际行为不符是最坏的坑 ⇒ 直接拒绝这个组合,让操作者明确选。
func TestConfigValidate_acpx不支持ask(t *testing.T) {
	c := Config{Transport: TransportACPX, Agent: "mcode", Permission: PermissionAsk}
	err := c.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "非交互")
	assert.Contains(t, err.Error(), "stdio", "错误信息要给出出路")

	// 其它档位可以
	for _, perm := range []string{PermissionApproveReads, PermissionApproveAll, PermissionDeny} {
		c2 := Config{Transport: TransportACPX, Agent: "mcode", Permission: perm}
		assert.NoError(t, c2.Validate(), "授权档 %s 应当可用", perm)
	}

	// stdio 后端可以逐次确认
	c3 := Config{Transport: TransportStdio, Command: "mcode acp", Permission: PermissionAsk}
	assert.NoError(t, c3.Validate(), "stdio 后端才是能逐次确认的那个")
}
