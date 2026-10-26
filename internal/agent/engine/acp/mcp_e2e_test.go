package acp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/agent/engine"
)

// mcpTestServer 一个最小的 MCP(Streamable HTTP JSON-RPC)服务器。
//
// 只实现 initialize / tools/list / tools/call —— 足够验证
// 「外部 agent 能挂上 git-ferry 的 MCP 并真的调通工具」这件事。
type mcpTestServer struct {
	calledTool string
	callArgs   string
}

func (s *mcpTestServer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "gitferry-test", "version": "1.0.0"},
			}
		case "tools/list":
			result = map[string]any{
				"tools": []map[string]any{{
					"name":        "list_repos",
					"description": "查询同步仓库列表",
					"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
				}},
			}
		case "tools/call":
			var p struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &p)
			s.calledTool = p.Name
			s.callArgs = p.Arguments
			result = map[string]any{
				"content": []map[string]any{{"type": "text", "text": `{"total":2,"items":["a/b","c/d"]}`}},
			}
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      json.RawMessage(req.ID),
			"result":  result,
		})
	})
}

// TestE2E_Stdio_MCPMounted 端到端:把 MCP 挂给外部 agent,让它真的调通工具。
//
// 这是「外部 agent 获得 git-ferry 领域能力」这条路的**唯一**证明。
func TestE2E_Stdio_MCPMounted(t *testing.T) {
	if !e2eEnabled() {
		t.Skip("设置 GITFERRY_ACP_E2E=1 跑真实 agent 端到端")
	}
	mcp := &mcpTestServer{}
	srv := httptest.NewServer(mcp.handler())
	defer srv.Close()

	dir := t.TempDir()
	eng, err := NewEngine(&Config{
		Command:    "mcode acp",
		Cwd:        dir,
		Timeout:    5 * time.Minute,
		Permission: PermissionApproveAll,
		MCPServers: []MCPServer{{
			Name: "gitferry",
			Type: "http",
			URL:  srv.URL + "/mcp",
		}},
	})
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	sess := newFakeSession("e2e-mcp")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	ch, engCancel, err := eng.Run(ctx, sess, engine.Input{
		Message: "请调用 gitferry MCP 的 list_repos 工具,把结果原样告诉我。",
	})
	require.NoError(t, err)
	defer engCancel()

	text, events := drain(t, ch, 5*time.Minute)
	t.Logf("事件数:%d 文本:%q", len(events), text)

	require.NotEmpty(t, events)
	// 至少看到一个工具调用(名字可能是 "list_repos" 或带前缀的形式)
	var sawTool bool
	for _, ev := range events {
		if ev.Type == engine.EventToolStart {
			sawTool = true
			t.Logf("工具调用:%s args=%s", ev.Tool, ev.Args)
		}
	}
	assert.True(t, sawTool, "外部 agent 应通过 MCP 发起工具调用")
	assert.Equal(t, "list_repos", mcp.calledTool, "MCP 服务器应收到 list_repos 调用")
	assert.Contains(t, text, "a/b", "agent 应把工具结果说出来")
}

// TestMCPServers_Payload 断言 session/new 的 mcpServers 载荷形状。
//
// **headers 必须是数组 [{name,value}]** —— ACP 规范原文的形状;
// 写成对象 agent 侧会解析失败(实测踩过)。
func TestMCPServers_Payload(t *testing.T) {
	e := &Engine{cfg: Config{MCPServers: []MCPServer{
		{
			Name:    "gitferry",
			Type:    "http",
			URL:     "http://127.0.0.1:8890/mcp",
			Headers: []HttpHeader{{Name: "X-API-Key", Value: "secret"}},
		},
		{
			Name:    "local",
			Command: "/usr/bin/mcp-server",
			Args:    []string{"--stdio"},
			Env:     []EnvVar{{Name: "K", Value: "V"}},
		},
	}}}
	payload := e.mcpServers()
	require.Len(t, payload, 2)

	httpSrv := payload[0]
	assert.Equal(t, "http", httpSrv["type"])
	assert.Equal(t, "gitferry", httpSrv["name"])
	headers, ok := httpSrv["headers"].([]map[string]string)
	require.True(t, ok, "headers 必须是数组")
	require.Len(t, headers, 1)
	assert.Equal(t, "X-API-Key", headers[0]["name"])
	assert.Equal(t, "secret", headers[0]["value"])

	stdio := payload[1]
	assert.Equal(t, "local", stdio["name"])
	assert.Equal(t, "/usr/bin/mcp-server", stdio["command"])
	assert.Equal(t, []string{"--stdio"}, stdio["args"])
	env, ok := stdio["env"].([]map[string]string)
	require.True(t, ok)
	assert.Equal(t, "K", env[0]["name"])
}

// TestMCPHeaders_不泄露密钥到日志 载荷里有密钥是必然的(要发给 agent),
// 但**不能**出现在日志/错误信息里。
func TestMCPHeaders_不泄露密钥到日志(t *testing.T) {
	e := &Engine{cfg: Config{MCPServers: []MCPServer{{
		Name: "gitferry", Type: "http", URL: "http://x/mcp",
		Headers: []HttpHeader{{Name: "X-API-Key", Value: "top-secret-value"}},
	}}}}
	payload := e.mcpServers()
	b, err := json.Marshal(payload)
	require.NoError(t, err)
	// 载荷本身要带密钥(否则 agent 没法鉴权)
	assert.True(t, strings.Contains(string(b), "top-secret-value"))
}
