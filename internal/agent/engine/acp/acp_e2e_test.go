package acp

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/agent/engine"
)

// 真实端到端:需要本机装了 acpx 与对应 agent。
//
// 默认跳过(单测不该依赖外部 agent);实测时:
//
//	GITFERRY_ACP_E2E=1 go test ./internal/agent/engine/acp/ -run E2E -v
func e2eEnabled() bool { return os.Getenv("GITFERRY_ACP_E2E") == "1" }

// drain 收完事件流,返回文本与事件。
func drain(t *testing.T, ch <-chan engine.Event, timeout time.Duration) (string, []engine.Event) {
	t.Helper()
	var (
		text   strings.Builder
		events []engine.Event
	)
	deadline := time.After(timeout)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return text.String(), events
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

// TestE2E_Acpx_RealAgent 用真实 acpx + mcode 跑一轮。
func TestE2E_Acpx_RealAgent(t *testing.T) {
	if !e2eEnabled() {
		t.Skip("设置 GITFERRY_ACP_E2E=1 跑真实 agent 端到端")
	}
	dir := t.TempDir()
	eng, err := NewAcpxEngine(&Config{
		Agent:   "mcode",
		Cwd:     dir,
		Timeout: 3 * time.Minute,
	})
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	sess := newFakeSession("e2e-" + t.Name())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	ch, engCancel, err := eng.Run(ctx, sess, engine.Input{Message: "只回复两个字:你好"})
	require.NoError(t, err)
	defer engCancel()

	text, events := drain(t, ch, 3*time.Minute)
	t.Logf("事件:%v", events)
	assert.Contains(t, text, "你好", "agent 应回「你好」")
	assert.Equal(t, engine.EventDone, events[len(events)-1].Type)
}

// TestE2E_Stdio_RealAgent 用原生 ACP(直连 mcode acp)跑一轮。
func TestE2E_Stdio_RealAgent(t *testing.T) {
	if !e2eEnabled() {
		t.Skip("设置 GITFERRY_ACP_E2E=1 跑真实 agent 端到端")
	}
	dir := t.TempDir()
	eng, err := NewEngine(&Config{
		Command: "mcode acp",
		Cwd:     dir,
		Timeout: 3 * time.Minute,
	})
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	sess := newFakeSession("e2e-" + t.Name())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	ch, engCancel, err := eng.Run(ctx, sess, engine.Input{Message: "只回复两个字:你好"})
	require.NoError(t, err)
	defer engCancel()

	text, events := drain(t, ch, 3*time.Minute)
	t.Logf("事件:%v", events)
	assert.Contains(t, text, "你好", "agent 应回「你好」")
	assert.Equal(t, engine.EventDone, events[len(events)-1].Type)
}
