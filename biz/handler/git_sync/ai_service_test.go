package git_sync

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	hertzserver "github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/agent"
	"github.com/yi-nology/git-ferry/internal/agent/tools"
	"github.com/yi-nology/git-ferry/internal/agent/tools/toolstest"
)

// ===== 未启用降级(ut 即可,无流式) =====

func TestAIStatus_Disabled(t *testing.T) {
	SetAgentRunner(func() *agent.Runner { return nil })
	h := hertzserver.Default()
	h.GET("/api/v1/ai/status", AIStatus)
	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/v1/ai/status", nil)
	assert.Equal(t, http.StatusNotImplemented, w.Code)
	assert.Contains(t, w.Body.String(), "ai_disabled")
}

func TestAIChat_Disabled(t *testing.T) {
	SetAgentRunner(func() *agent.Runner { return nil })
	h := hertzserver.Default()
	h.POST("/api/v1/ai/chat", AIChat)
	w := ut.PerformRequest(h.Engine, http.MethodPost, "/api/v1/ai/chat",
		&ut.Body{Body: strings.NewReader(`{"message":"hi"}`), Len: 13},
		ut.Header{Key: "Content-Type", Value: "application/json"})
	assert.Equal(t, http.StatusNotImplemented, w.Code)
}

// ===== SSE 流式(起真端口,HijackWriter 不走 ut 记录器) =====

func startAITestServer(t *testing.T, runner *agent.Runner) string {
	t.Helper()
	SetAgentRunner(func() *agent.Runner { return runner })
	SetAPIKey("test-key")

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())

	h := hertzserver.Default(hertzserver.WithHostPorts(fmt.Sprintf("127.0.0.1:%d", port)))
	h.GET("/api/v1/ai/status", AIStatus)
	h.POST("/api/v1/ai/chat", AIChat)
	go h.Spin()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = h.Shutdown(ctx)
	})

	// 等端口就绪
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			_ = conn.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

func postChat(t *testing.T, base, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, base+"/api/v1/ai/chat", bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", "test-key")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func newTextRunner(t *testing.T, content string) *agent.Runner {
	t.Helper()
	fm := toolstest.NewFakeModel()
	fm.Append(toolstest.FakeStep{Content: content})
	r, err := agent.NewRunnerWithModel(&agent.Config{MaxConcurrentChats: 2}, fm, tools.NewRegistry(toolstest.NewMock()))
	require.NoError(t, err)
	return r
}

func TestAIChat_SSEFlow(t *testing.T) {
	base := startAITestServer(t, newTextRunner(t, "答复内容"))

	resp := postChat(t, base, `{"message":"hi"}`)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/event-stream")

	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(resp.Body)
	require.NoError(t, err)
	body := buf.String()

	assert.Contains(t, body, "event:start")
	assert.Contains(t, body, "event:delta")
	assert.Contains(t, body, "答复内容")
	assert.Contains(t, body, "event:done")
}

func TestAIChat_SessionContinuity(t *testing.T) {
	base := startAITestServer(t, newTextRunner(t, "第一次回复"))

	resp := postChat(t, base, `{"message":"one"}`)
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(resp.Body)
	require.NoError(t, err)

	// 从 start 事件取 session_id
	var sessionID string
	for _, frame := range strings.Split(buf.String(), "\n\n") {
		if strings.Contains(frame, "event:start") {
			for _, line := range strings.Split(frame, "\n") {
				if strings.HasPrefix(line, "data:") {
					sessionID = betweenQuotes(line, `"session_id":"`, `"`)
				}
			}
		}
	}
	require.NotEmpty(t, sessionID)

	// 带会话继续对话;不存在的会话 404
	resp2 := postChat(t, base, fmt.Sprintf(`{"session_id":%q,"message":"two"}`, sessionID))
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	resp3 := postChat(t, base, `{"session_id":"not-exist","message":"x"}`)
	assert.Equal(t, http.StatusNotFound, resp3.StatusCode)
}

func TestAIChat_ConfirmFlow(t *testing.T) {
	fm := toolstest.NewFakeModel()
	fm.Append(toolstest.FakeStep{ToolName: "run_task", ToolArgs: `{"task_key":"t1"}`})
	fm.Append(toolstest.FakeStep{Content: "等待确认"})
	mock := toolstest.NewMock()
	r, err := agent.NewRunnerWithModel(&agent.Config{MaxConcurrentChats: 2}, fm, tools.NewRegistry(mock))
	require.NoError(t, err)
	base := startAITestServer(t, r)

	// 第一轮:模型调 run_task → tool_confirm
	resp := postChat(t, base, `{"message":"同步 t1"}`)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(resp.Body)
	require.NoError(t, err)
	body := buf.String()

	assert.Contains(t, body, "event:tool_start")
	assert.Contains(t, body, "event:tool_confirm")
	assert.Contains(t, body, "event:done")
	assert.Empty(t, mock.RunTaskKey(), "未确认不得执行")

	var token string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "data:") && strings.Contains(line, "tool_confirm") {
			token = betweenQuotes(line, `"token":"`, `"`)
		}
	}
	require.NotEmpty(t, token, "tool_confirm 应携带令牌")

	// 取会话继续
	var sessionID string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "data:") && strings.Contains(line, `"session_id":"`) {
			if v := betweenQuotes(line, `"session_id":"`, `"`); v != "" {
				sessionID = v
			}
		}
	}
	require.NotEmpty(t, sessionID)

	// 第二轮:带确认令牌直达执行
	resp2 := postChat(t, base, fmt.Sprintf(
		`{"session_id":%q,"message":" ","confirmed_tool_call":{"tool":"run_task","token":%q}}`, sessionID, token))
	require.Equal(t, http.StatusOK, resp2.StatusCode)
	buf2 := new(bytes.Buffer)
	_, err = buf2.ReadFrom(resp2.Body)
	require.NoError(t, err)
	body2 := buf2.String()

	assert.Contains(t, body2, "event:tool_end")
	// result 是字符串字段,内层 JSON 被转义
	assert.Contains(t, body2, "同步已执行完成")
	assert.Equal(t, "t1", mock.RunTaskKey(), "确认后应真实执行")
}

func TestAIStatus_Enabled(t *testing.T) {
	base := startAITestServer(t, newTextRunner(t, "ok"))
	req, err := http.NewRequest(http.MethodGet, base+"/api/v1/ai/status", http.NoBody)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", "test-key")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	assert.Contains(t, buf.String(), `"enabled":true`)
}

// betweenQuotes 从 s 中截取 key 后引号包住的值;找不到返回空。
func betweenQuotes(s, key, end string) string {
	i := strings.Index(s, key)
	if i < 0 {
		return ""
	}
	rest := s[i+len(key):]
	j := strings.Index(rest, end)
	if j < 0 {
		return ""
	}
	return rest[:j]
}
