package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/agent"
)

// TestMCPServersFor_本服务加额外 挂载清单 = 本服务自己的 /mcp + 额外的 MCP 服务器。
func TestMCPServersFor_本服务加额外(t *testing.T) {
	t.Setenv("GITFERRY_AI_MCP_API_KEY", "ro-key")
	t.Setenv("EXTRA_MCP_KEY", "Bearer secret")

	ext := agent.ExternalConfig{ //nolint:gosec // G101:字段名含 Key,此处是环境变量名与测试值,不是凭据
		MCPEnabled:   true,
		MCPURL:       "http://127.0.0.1:9999/mcp",
		MCPAPIKeyEnv: "GITFERRY_AI_MCP_API_KEY",
		MCPServers: []agent.MCPServerConfig{
			{Name: "docs", Type: "http", URL: "https://docs.example.com/mcp", APIKeyEnv: "EXTRA_MCP_KEY"}, //nolint:gosec // G101:环境变量名,不是凭据
			{Name: "local", Type: "stdio", Command: "/usr/bin/mcp-server", Args: []string{"--stdio"}},
		},
	}
	list, err := mcpServersFor(&ext, nil)
	require.NoError(t, err)
	require.Len(t, list, 3)

	// 本服务自己:固定 X-API-Key
	assert.Equal(t, "gitferry", list[0].Name)
	require.Len(t, list[0].Headers, 1)
	assert.Equal(t, "X-API-Key", list[0].Headers[0].Name)
	assert.Equal(t, "ro-key", list[0].Headers[0].Value)

	// 额外服务器:默认 Authorization,值取环境变量里的完整头值
	assert.Equal(t, "docs", list[1].Name)
	require.Len(t, list[1].Headers, 1)
	assert.Equal(t, "Authorization", list[1].Headers[0].Name)
	assert.Equal(t, "Bearer secret", list[1].Headers[0].Value)

	// stdio 服务器:无认证头
	assert.Equal(t, "local", list[2].Name)
	assert.Empty(t, list[2].Headers)
	assert.Equal(t, "/usr/bin/mcp-server", list[2].Command)
}

// TestMCPServersFor_缺环境变量直接报错 密钥只走环境变量;
// 变量没设时不能静默发一个空认证头过去(那等于没认证,还让人以为配好了)。
func TestMCPServersFor_缺环境变量直接报错(t *testing.T) {
	ext := agent.ExternalConfig{
		MCPServers: []agent.MCPServerConfig{
			{Name: "x", Type: "http", URL: "https://x/mcp", APIKeyEnv: "GITFERRY_NOT_SET_XYZ"}, //nolint:gosec // G101:环境变量名,不是凭据
		},
	}
	_, err := mcpServersFor(&ext, nil)
	assert.ErrorContains(t, err, "GITFERRY_NOT_SET_XYZ")
}

// TestMCPServersFor_缺name报错 没有名字的服务器在 agent 侧没法识别。
func TestMCPServersFor_缺name报错(t *testing.T) {
	ext := agent.ExternalConfig{
		MCPServers: []agent.MCPServerConfig{{Type: "http", URL: "https://x/mcp"}},
	}
	_, err := mcpServersFor(&ext, nil)
	assert.ErrorContains(t, err, "name")
}
