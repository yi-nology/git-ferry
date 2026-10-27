package acp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWriteMCPConfig_acpx形状 **形状以 acpx 0.21.0 实测源码为准**:
//   - `mcpServers` 是**数组**(acpx 的 parseMcpServers 明确 expected array);
//   - `headers` / `env` 是**数组** `[{name,value}]`;
//   - `type` 不写会被按 stdio 解析 ⇒ http 端点必须显式写。
//
// 写错的后果很隐蔽:acpx **不报错**,但工具列表是空的
// ⇒ 外部 agent 找不到工具 ⇒ 满环境自己找 ⇒ 撞超时。
func TestWriteMCPConfig_acpx形状(t *testing.T) {
	path, cleanup, err := writeMCPConfig([]MCPServer{
		{
			Name: "gitferry",
			Type: "http",
			URL:  "http://127.0.0.1:8890/mcp",
			Headers: []HttpHeader{
				{Name: "X-API-Key", Value: "ro-key-123"},
			},
		},
		{
			Name:    "local",
			Command: "/usr/bin/mcp-server",
			Args:    []string{"--stdio"},
			Env:     []EnvVar{{Name: "TOKEN", Value: "t"}},
		},
	})
	require.NoError(t, err)
	defer cleanup()
	require.NotEmpty(t, path)

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))

	// mcpServers 必须是数组(不是对象映射)
	servers, ok := doc["mcpServers"].([]any)
	require.True(t, ok, "mcpServers 必须是数组(acpx: expected array),实际 %T", doc["mcpServers"])
	require.Len(t, servers, 2)

	first := servers[0].(map[string]any)
	assert.Equal(t, "gitferry", first["name"])
	assert.Equal(t, "http", first["type"], "type 不写会被按 stdio 解析 ⇒ 工具列表为空")
	assert.Equal(t, "http://127.0.0.1:8890/mcp", first["url"])
	// headers 必须是数组 [{name,value}]
	headers, ok := first["headers"].([]any)
	require.True(t, ok, "headers 必须是数组(写成对象 ⇒ 不报错但工具列表为空)")
	require.Len(t, headers, 1)
	h := headers[0].(map[string]any)
	assert.Equal(t, "X-API-Key", h["name"])
	assert.Equal(t, "ro-key-123", h["value"])

	second := servers[1].(map[string]any)
	assert.Equal(t, "/usr/bin/mcp-server", second["command"])
	assert.Equal(t, []any{"--stdio"}, second["args"])
	env, ok := second["env"].([]any)
	require.True(t, ok, "env 必须是数组")
	require.Len(t, env, 1)
}

// TestWriteMCPConfig_权限与清理 文件里躺着只读 key 的专用凭据 ⇒
// 目录 0700、文件 0600;清理函数要真删掉(不留凭据在盘上)。
func TestWriteMCPConfig_权限与清理(t *testing.T) {
	path, cleanup, err := writeMCPConfig([]MCPServer{
		{Name: "gitferry", Type: "http", URL: "http://x/mcp",
			Headers: []HttpHeader{{Name: "X-API-Key", Value: "secret"}}},
	})
	require.NoError(t, err)

	dir := filepath.Dir(path)
	di, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), di.Mode().Perm(), "临时目录应 0700")

	fi, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm(), "配置文件应 0600")

	cleanup()
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "清理后文件不该还在(凭据不能留在盘上)")
}

// TestWriteMCPConfig_没配MCP不写文件 没配 MCP 时不该产生临时文件
// (acpx 用它自己的项目/全局配置)。
func TestWriteMCPConfig_没配MCP不写文件(t *testing.T) {
	path, cleanup, err := writeMCPConfig(nil)
	require.NoError(t, err)
	defer cleanup()
	assert.Empty(t, path)
}
