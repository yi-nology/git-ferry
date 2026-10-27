package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yi-nology/git-ferry/internal/agent"
	"github.com/yi-nology/git-ferry/internal/agent/engine"
	"github.com/yi-nology/git-ferry/internal/agent/engine/acp"
	"github.com/yi-nology/git-ferry/internal/agent/tools/toolstest"
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
	list, err := mcpServersFor(ext.MCPEnabled, ext.MCPURL, ext.MCPAPIKeyEnv, ext.MCPServers, nil)
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
	_, err := mcpServersFor(ext.MCPEnabled, ext.MCPURL, ext.MCPAPIKeyEnv, ext.MCPServers, nil)
	assert.ErrorContains(t, err, "GITFERRY_NOT_SET_XYZ")
}

// TestMCPServersFor_缺name报错 没有名字的服务器在 agent 侧没法识别。
func TestMCPServersFor_缺name报错(t *testing.T) {
	ext := agent.ExternalConfig{
		MCPServers: []agent.MCPServerConfig{{Type: "http", URL: "https://x/mcp"}},
	}
	_, err := mcpServersFor(ext.MCPEnabled, ext.MCPURL, ext.MCPAPIKeyEnv, ext.MCPServers, nil)
	assert.ErrorContains(t, err, "name")
}

// TestResolveProfile_列表与旧行为 列表为空 ⇒ 单 profile 字段(旧行为,一个不变);
// 列表非空 ⇒ 以列表为准(Current 选中的那个)。
func TestResolveProfile_列表与旧行为(t *testing.T) {
	// 旧行为:没有列表
	old := &agent.Config{
		Backend: agent.BackendACPX,
		External: agent.ExternalConfig{
			Agent: "mcode", Cwd: "/ws", Permission: "ask",
		},
	}
	p := old.ResolveProfile("")
	assert.Equal(t, "mcode", p.Agent)
	assert.Equal(t, agent.BackendACPX, p.Transport, "没写 transport 时跟随 ai.backend")
	assert.Equal(t, "/ws", p.Cwd)

	// 列表模式:Current 选中
	list := &agent.Config{
		Backend: agent.BackendACPX,
		External: agent.ExternalConfig{
			Current: "codex",
			Agents: []agent.ExternalAgentConfig{
				{Name: "claude", Transport: agent.BackendStdio, Command: "claude-code-acp", Cwd: "/a"},
				{Name: "codex", Transport: agent.BackendACPX, Agent: "codex", Cwd: "/b"},
			},
		},
	}
	p2 := list.ResolveProfile("")
	assert.Equal(t, "codex", p2.Name, "应选 Current 指定的")
	assert.Equal(t, agent.BackendACPX, p2.Transport)

	// 按名字精确命中
	p3, ok := list.ResolveProfileByName("claude")
	require.True(t, ok)
	assert.Equal(t, agent.BackendStdio, p3.Transport)
	assert.Equal(t, "claude-code-acp", p3.Command)

	// 没命中
	_, ok = list.ResolveProfileByName("nope")
	assert.False(t, ok)
}

// TestDelegatePermissionFor_ask映射成approve_reads 一次性执行弹不出授权卡,
// `ask` 在委派场景没有对应物 ⇒ 映射成 approve_reads。
//
// **这个映射是必须的**:acpx 是非交互通道,`ask` 会被校验直接拒掉 ⇒
// 不映射的话,默认配置下**委派执行器根本装不上**
// (表现为 ai_code_delegate 报「未配置」,像没配过一样)。
func TestDelegatePermissionFor_ask映射成approve_reads(t *testing.T) {
	assert.Equal(t, acp.PermissionApproveReads, delegatePermissionFor(""))
	assert.Equal(t, acp.PermissionApproveReads, delegatePermissionFor(acp.PermissionAsk))
	assert.Equal(t, acp.PermissionApproveAll, delegatePermissionFor(acp.PermissionApproveAll))
	assert.Equal(t, acp.PermissionDeny, delegatePermissionFor(acp.PermissionDeny))
}

// TestAttachDelegator_默认配置也要装得上 默认权限档(空)也必须能装配委派执行器,
// 否则 ai_code_delegate 会一直报「未配置」。
func TestAttachDelegator_默认配置也要装得上(t *testing.T) {
	cfg := &agent.Config{
		Backend: agent.BackendInternal,
		External: agent.ExternalConfig{
			Agent: "mcode",
		},
	}
	eng, err := agent.NewEinoEngine(cfg, "key", toolstest.NewMock())
	require.NoError(t, err)
	attachDelegator(cfg, eng, nil)

	// **必须真的装上了** —— 装不上时 ai_code_delegate 会报「未配置」,
	// 看起来像用户没配,实际是装配失败(例如授权档没映射对)。
	probe, ok := eng.(engine.DelegatorProbe)
	require.True(t, ok)
	assert.True(t, probe.HasDelegator(), "默认配置下委派执行器必须装得上")
}
