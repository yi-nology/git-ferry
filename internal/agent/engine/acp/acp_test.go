package acp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/agent/engine"
)

// fixtureLines 读实测夹具(一行一个 JSON-RPC 报文)。
func fixtureLines(t *testing.T, name string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	var out []string
	for _, l := range strings.Split(string(data), "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	require.NotEmpty(t, out)
	return out
}

func TestLineReader_SkipsNonJSONLines(t *testing.T) {
	// acpx 实测会往 stdout 打横幅行;解析器必须跳过而不是崩。
	in := "[acpx] session gftest (uuid) · /tmp/ws · agent mcode\n" +
		`{"jsonrpc":"2.0","method":"session/update","params":{}}` + "\n" +
		"\n" +
		`{"jsonrpc":"2.0","id":1,"result":{}}` + "\n"
	r := newLineReader(strings.NewReader(in))

	m1, err := r.next()
	require.NoError(t, err)
	require.NotNil(t, m1)
	assert.Equal(t, "session/update", m1.Method)

	m2, err := r.next()
	require.NoError(t, err)
	require.NotNil(t, m2)
	assert.True(t, m2.isResponse())

	m3, err := r.next()
	require.NoError(t, err)
	assert.Nil(t, m3, "流结束应返回 nil")
	assert.Equal(t, 1, r.skipped, "横幅行应被计数,不静默吞掉")
}

func TestRPCID_Normalizes(t *testing.T) {
	assert.Equal(t, "1", rpcID(json.RawMessage("1")))
	assert.Equal(t, "abc", rpcID(json.RawMessage(`"abc"`)))
	assert.Equal(t, "", rpcID(json.RawMessage("null")))
	assert.Equal(t, "", rpcID(nil))
}

// TestMapUpdate_MeasuredShapes 用**实测报文形状**钉住映射。
func TestMapUpdate_MeasuredShapes(t *testing.T) {
	m := newUpdateMapper()

	// 实测形状:agent_message_chunk
	ev := m.mapUpdate(&sessionUpdate{
		SessionUpdate: "agent_message_chunk",
		MessageID:     "64c8f2b8-7ae4-4491-a7a1-49e052a0e400",
		Content:       &contentBlock{Type: "text", Text: "你好"},
	})
	require.NotNil(t, ev)
	assert.Equal(t, engine.EventDelta, ev.Type)
	assert.Equal(t, "你好", ev.Content)

	// 实测形状:tool_call 带 name/title/kind
	ev = m.mapUpdate(&sessionUpdate{
		SessionUpdate: "tool_call",
		ToolCallID:    "call_01a17616fbea74909d2808bd",
		Title:         "read",
		Name:          "read",
		Kind:          "read",
	})
	require.NotNil(t, ev)
	assert.Equal(t, engine.EventToolStart, ev.Type)
	assert.Equal(t, "read", ev.Tool)

	// 实测形状:tool_call_update 中间态只有 toolCallId,没有 name/title
	ev = m.mapUpdate(&sessionUpdate{
		SessionUpdate: "tool_call_update",
		ToolCallID:    "call_01a17616fbea74909d2808bd",
	})
	assert.Nil(t, ev, "中间态不产事件")

	// 实测形状:tool_call_update 终态;工具名要能从 tool_call 记住
	ev = m.mapUpdate(&sessionUpdate{
		SessionUpdate: "tool_call_update",
		ToolCallID:    "call_01a17616fbea74909d2808bd",
		Status:        "completed",
		RawOutput:     json.RawMessage(`{"content":[{"type":"text","text":"file body"}]}`),
	})
	require.NotNil(t, ev)
	assert.Equal(t, engine.EventToolEnd, ev.Type)
	assert.Equal(t, "read", ev.Tool, "工具名应从 tool_call 记住")
	assert.Equal(t, "file body", ev.Result)

	// 未知类型忽略
	assert.Nil(t, m.mapUpdate(&sessionUpdate{SessionUpdate: "available_commands_update"}))
	assert.Nil(t, m.mapUpdate(&sessionUpdate{SessionUpdate: "usage_update", Used: 10}))
	assert.Nil(t, m.mapUpdate(&sessionUpdate{SessionUpdate: "session_info_update"}))
}

// TestMapUpdate_NativeFixture 把**实测的原生 ACP 报文流**整体过一遍映射。
func TestMapUpdate_NativeFixture(t *testing.T) {
	lines := fixtureLines(t, "acp_native.ndjson")
	m := newUpdateMapper()

	var (
		deltas    []string
		toolStart []string
		toolEnd   []string
	)
	for _, l := range lines {
		var msg rpcMessage
		require.NoError(t, json.Unmarshal([]byte(l), &msg))
		if !msg.isNotification() || msg.Method != "session/update" {
			continue
		}
		var params struct {
			Update *sessionUpdate `json:"update"`
		}
		require.NoError(t, json.Unmarshal(msg.Params, &params))
		ev := m.mapUpdate(params.Update)
		if ev == nil {
			continue
		}
		switch ev.Type {
		case engine.EventDelta:
			deltas = append(deltas, ev.Content)
		case engine.EventToolStart:
			toolStart = append(toolStart, ev.Tool)
		case engine.EventToolEnd:
			toolEnd = append(toolEnd, ev.Tool)
		}
	}
	assert.NotEmpty(t, deltas, "实测夹具里应有文本增量")
	assert.NotEmpty(t, toolStart, "实测夹具里应有工具调用")
	assert.NotEmpty(t, toolEnd, "实测夹具里应有工具终态")
	// 工具名在 tool_call_update 里不带,必须从 tool_call 记住 ⇒ 两边名字要对得上
	require.Len(t, toolStart, len(toolEnd))
	for i := range toolStart {
		assert.Equal(t, toolStart[i], toolEnd[i], "tool_end 的工具名应与 tool_start 一致")
	}
}

// fakeAcpx 造一个只会回放夹具的假 acpx。
func fakeAcpx(t *testing.T, fixture string) string {
	t.Helper()
	// 夹具路径必须是**绝对路径**:引擎会把 cmd.Dir 设成工作目录。
	abs, err := filepath.Abs(filepath.Join("testdata", fixture))
	require.NoError(t, err)
	dir := t.TempDir()
	bin := filepath.Join(dir, "acpx")
	script := "#!/bin/sh\ncat '" + abs + "'\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o700)) //nolint:gosec // 测试夹具脚本
	return bin
}

// TestAcpxEngine_Run 实测夹具驱动的端到端:acpx 输出 → 事件流。
func TestAcpxEngine_Run(t *testing.T) {
	dir := t.TempDir()
	eng, err := NewAcpxEngine(&Config{
		Binary:     fakeAcpx(t, "acpx_exec.ndjson"),
		Agent:      "mcode",
		Cwd:        dir,
		Permission: PermissionApproveReads, // acpx 非交互,ask 档不可用
	})
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	sess := newFakeSession("sess-1")
	ch, cancel, err := eng.Run(context.Background(), sess, engine.Input{Message: "只回复两个字:你好"})
	require.NoError(t, err)
	defer cancel()

	var events []engine.Event
	timeout := time.After(15 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				require.NotEmpty(t, events)
				// 文本增量
				var text strings.Builder
				for _, e := range events {
					if e.Type == engine.EventDelta {
						text.WriteString(e.Content)
					}
				}
				assert.Equal(t, "你好", text.String())
				// 收尾是 done
				assert.Equal(t, engine.EventDone, events[len(events)-1].Type)
				return
			}
			events = append(events, ev)
		case <-timeout:
			t.Fatal("等待事件超时")
		}
	}
}

func TestConfigValidate(t *testing.T) {
	// 缺 agent/command
	c := Config{Transport: TransportACPX}
	assert.ErrorContains(t, c.Validate(), "agent")

	// stdio 缺 command
	c2 := Config{Transport: TransportStdio}
	assert.ErrorContains(t, c2.Validate(), "command")

	// 未知 transport
	c3 := Config{Transport: "carrier-pigeon"}
	assert.ErrorContains(t, c3.Validate(), "transport")

	// 未知权限档
	c4 := Config{Transport: TransportACPX, Agent: "mcode", Permission: "whatever"}
	assert.ErrorContains(t, c4.Validate(), "permission")

	// 正常:acpx 要显式选一个能兑现的授权档(ask 弹不出授权卡,见 stdio_test.go)
	c5 := Config{Transport: TransportACPX, Agent: "mcode", Permission: PermissionApproveReads}
	require.NoError(t, c5.Validate())
	assert.Equal(t, acpxBinary, c5.Binary)

	// stdio 后端才是能逐次确认的那个
	c6 := Config{Transport: TransportStdio, Command: "mcode acp"}
	require.NoError(t, c6.Validate())
	assert.Equal(t, PermissionAsk, c6.Permission, "默认档是 ask")
}

func TestFsHandler_Sandbox(t *testing.T) {
	dir := t.TempDir()
	h, err := newFSHandler(dir)
	require.NoError(t, err)

	// 根内可读
	inside := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(inside, []byte("hello"), 0o600))
	res, err := h.readTextFile(json.RawMessage(`{"path":"` + inside + `"}`))
	require.NoError(t, err)
	assert.Contains(t, res.(map[string]any)["content"], "hello")

	// 根外拒绝
	outside := filepath.Join(filepath.Dir(dir), "outside.txt")
	_, err = h.readTextFile(json.RawMessage(`{"path":"` + outside + `"}`))
	assert.ErrorContains(t, err, "越界")

	// 相对路径按 root 解析
	rel, err := h.readTextFile(json.RawMessage(`{"path":"a.txt"}`))
	require.NoError(t, err)
	assert.Contains(t, rel.(map[string]any)["content"], "hello")

	// 写文件
	require.NoError(t, h.writeTextFile(json.RawMessage(`{"path":"b.txt","content":"x"}`)))
	got, err := os.ReadFile(filepath.Join(dir, "b.txt"))
	require.NoError(t, err)
	assert.Equal(t, "x", string(got))
}

func TestSplitCommand(t *testing.T) {
	assert.Equal(t, []string{"mcode", "acp"}, splitCommand("mcode acp"))
	assert.Equal(t, []string{"mcode", "acp", "--x"}, splitCommand("  mcode   acp --x  "))
	assert.Equal(t, []string{"./bin/dev acp", "--p"}, splitCommand("'./bin/dev acp' --p"))
	assert.Nil(t, splitCommand("   "))
}

func TestIsNoSessionError(t *testing.T) {
	re := &rpcError{Code: -32002, Message: "⚠ No acpx session found", Data: json.RawMessage(`{"acpxCode":"NO_SESSION"}`)}
	assert.True(t, isNoSessionError(re))
	assert.False(t, isNoSessionError(&rpcError{Code: -1, Message: "别的错"}))
	assert.False(t, isNoSessionError(nil))
}

// TestEvictOldest_不打断在飞的回合 回收进程会把在跑的回合打断
// (用户看到「工具调用到一半断了」)。有回合在飞时宁可不回收。
func TestEvictOldest_不打断在飞的回合(t *testing.T) {
	e := &Engine{cfg: Config{MaxProcs: 1}, procs: make(map[string]*sessionProc)}
	busy := &sessionProc{}
	busy.beginTurn()
	e.procs["busy"] = busy

	e.evictOldest()
	_, still := e.procs["busy"]
	assert.True(t, still, "有回合在飞的进程不该被回收")

	// 回合结束后就该能被回收
	busy.endTurn()
	e.evictOldest()
	_, gone := e.procs["busy"]
	assert.False(t, gone, "空闲进程应被回收")
}

// TestReady_静态就位性 Ready 只答「装没装」,不该启动进程(慢)。
func TestReady_静态就位性(t *testing.T) {
	e := &Engine{cfg: Config{Command: "definitely-not-a-real-command-xyz"}}
	ok, msg := e.Ready()
	assert.False(t, ok)
	assert.Contains(t, msg, "找不到")
}

// TestFsHandler_符号链接逃逸被拒 只做 filepath.Clean 是不够的:
// 工作区里一个指向 ~/.ssh/id_rsa 的软链,路径看起来仍在根下,
// 实际读的是根外的文件 ⇒ 沙箱形同虚设。
func TestFsHandler_符号链接逃逸被拒(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "secret.txt")
	require.NoError(t, os.WriteFile(outside, []byte("secret"), 0o600))

	link := filepath.Join(root, "link.txt")
	require.NoError(t, os.Symlink(outside, link))

	h, err := newFSHandler(root)
	require.NoError(t, err)

	_, err = h.readTextFile(json.RawMessage(`{"path":"` + link + `"}`))
	assert.ErrorContains(t, err, "越界", "指向根外的软链必须被拒")

	// 写也不能借软链逃逸
	err = h.writeTextFile(json.RawMessage(`{"path":"` + link + `","content":"pwn"}`))
	assert.ErrorContains(t, err, "越界")
	// 原文件没被动过
	got, err := os.ReadFile(outside)
	require.NoError(t, err)
	assert.Equal(t, "secret", string(got))
}

// TestTerminalHandler_有上限 终端只在 agent 调 terminal/release 时才释放;
// agent 不释放的话这个 map 就一直涨 ⇒ 内存泄漏。给个上限,超了直接拒。
func TestTerminalHandler_有上限(t *testing.T) {
	h := newTerminalHandler(t.TempDir())
	h.max = 2

	for i := 0; i < 2; i++ {
		res, err := h.create(json.RawMessage(`{"command":"sleep","args":["5"]}`))
		require.NoError(t, err, "第 %d 个终端应当建得出来", i)
		require.NotNil(t, res)
	}
	_, err := h.create(json.RawMessage(`{"command":"sleep","args":["5"]}`))
	assert.ErrorContains(t, err, "上限", "超上限要明确报错,而不是悄悄回收 agent 正在用的终端")

	// 释放一个之后又能建
	h.mu.Lock()
	var id string
	for k := range h.terms {
		id = k
	}
	h.mu.Unlock()
	require.NotEmpty(t, id)
	_, err = h.release(json.RawMessage(`{"terminalId":"` + id + `"}`))
	require.NoError(t, err)
	_, err = h.create(json.RawMessage(`{"command":"sleep","args":["5"]}`))
	assert.NoError(t, err, "释放之后应能再建")
}
