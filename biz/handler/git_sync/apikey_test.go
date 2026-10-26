package git_sync

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestResolveAPIKeyRole_主key与带角色key 角色是**按 key 判定**的,不是按来源。
//
// 为什么要按 key:外部 coding agent 需要一个只读 key 去访问本服务的 MCP
// (MCP 路径上危险工具不弹确认),而主 key 是 admin 角色。
func TestResolveAPIKeyRole_主key与带角色key(t *testing.T) {
	// 隔离:本测试改全局状态,先存后恢复。
	oldKey, oldRole := serverAPIKey, serverAPIKeyRole
	t.Cleanup(func() {
		serverAPIKey, serverAPIKeyRole = oldKey, oldRole
		scopedMu.Lock()
		scopedKeys = map[string]Role{}
		aiMCPKey = ""
		scopedMu.Unlock()
	})

	SetAPIKey("main-admin-key")
	SetAPIKeyRole(string(RoleAdmin))
	SetScopedAPIKey("readonly-key", RoleReadonly)

	// 主 key → 全局角色
	role, ok := ResolveAPIKeyRole("main-admin-key")
	assert.True(t, ok)
	assert.Equal(t, RoleAdmin, role)

	// 带角色 key → 自己的角色
	role, ok = ResolveAPIKeyRole("readonly-key")
	assert.True(t, ok)
	assert.Equal(t, RoleReadonly, role)

	// 不认识的 key → 不通过
	_, ok = ResolveAPIKeyRole("nope")
	assert.False(t, ok)

	// 空 key → 不通过
	_, ok = ResolveAPIKeyRole("")
	assert.False(t, ok)
}

// TestSetAIMCPAPIKey_固定只读且替换旧key 外部 agent 的 MCP 专用 key
// 固定只读角色;重复设置要替换掉旧 key(不留孤儿凭据)。
func TestSetAIMCPAPIKey_固定只读且替换旧key(t *testing.T) {
	oldKey, oldRole := serverAPIKey, serverAPIKeyRole
	t.Cleanup(func() {
		serverAPIKey, serverAPIKeyRole = oldKey, oldRole
		scopedMu.Lock()
		scopedKeys = map[string]Role{}
		aiMCPKey = ""
		scopedMu.Unlock()
	})

	SetAPIKey("main")
	SetAPIKeyRole(string(RoleAdmin))

	SetAIMCPAPIKey("ai-key-1")
	role, ok := ResolveAPIKeyRole("ai-key-1")
	assert.True(t, ok)
	assert.Equal(t, RoleReadonly, role, "外部 agent 的 MCP key 固定只读")

	// 换 key:旧的立即失效
	SetAIMCPAPIKey("ai-key-2")
	_, ok = ResolveAPIKeyRole("ai-key-1")
	assert.False(t, ok, "旧 key 应立即失效")
	role, ok = ResolveAPIKeyRole("ai-key-2")
	assert.True(t, ok)
	assert.Equal(t, RoleReadonly, role)

	// 清除
	SetAIMCPAPIKey("")
	_, ok = ResolveAPIKeyRole("ai-key-2")
	assert.False(t, ok)
}

// TestSetScopedAPIKey_空值不注册 空 key 或空角色注册进去等于放行一切。
func TestSetScopedAPIKey_空值不注册(t *testing.T) {
	t.Cleanup(func() { scopedMu.Lock(); scopedKeys = map[string]Role{}; scopedMu.Unlock() })

	SetScopedAPIKey("", RoleAdmin)
	SetScopedAPIKey("k", "")
	assert.Equal(t, 0, ScopedAPIKeyCount())
}
