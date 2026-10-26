package git_sync

import (
	"crypto/subtle"
	"os"
	"sync"
)

// 壳层 API Key（公网默认鉴权）。不进 git-ferry-core。
// 启动时由 main 经 corebridge.LoadShellConfig 注入。

var serverAPIKey string
var serverAPIKeyRole string

// scopedKeys 附加的「带角色 API Key」。
//
// **为什么要它**:主 key 只有一个、角色也是全局的(所有持 key 的人同一角色)。
// 但外部 coding agent(ACP 后端)需要一个**只读** key 去访问 git-ferry 自己的
// /mcp —— MCP 路径上危险工具**不弹确认**(只过角色闸门),
// 把 admin 主 key 挂给外部 agent = 把「无需确认的写操作」交出去。
//
// 角色是**按 key 判定**的,不是按来源。
var (
	scopedMu    sync.RWMutex
	scopedKeys  = map[string]Role{}
	scopedOrder []string // 保持注册顺序,便于调试输出
)

// SetAPIKey 由壳层 main 在路由注册前调用。
func SetAPIKey(key string) {
	serverAPIKey = key
}

// GetAPIKey 返回当前配置的 API Key；空串表示未配置。
func GetAPIKey() string {
	return serverAPIKey
}

// SetAPIKeyRole 设置共享 API Key 的角色(admin/operator/readonly)。
func SetAPIKeyRole(role string) {
	serverAPIKeyRole = role
}

// GetAPIKeyRole 返回 API Key 角色;未设置时读环境变量,默认 admin(兼容旧行为)。
func GetAPIKeyRole() string {
	if serverAPIKeyRole != "" {
		return serverAPIKeyRole
	}
	if v := os.Getenv("GIT_SYNC_API_KEY_ROLE"); v != "" {
		return v
	}
	return string(RoleAdmin)
}

// SetScopedAPIKey 注册一个带角色的附加 API Key。
//
// 空 key 或空角色一律忽略(不注册)—— 注册一个空 key 等于放行一切。
func SetScopedAPIKey(key string, role Role) {
	if key == "" || role == "" {
		return
	}
	scopedMu.Lock()
	defer scopedMu.Unlock()
	if _, exists := scopedKeys[key]; !exists {
		scopedOrder = append(scopedOrder, key)
	}
	scopedKeys[key] = role
}

// ResolveAPIKeyRole 判定请求携带的 key 的角色。
//
// 返回 (role, ok):ok=false 表示这个 key 根本不认识(应 401)。
// 比较一律用常量时间,避免按字节比较泄漏前缀信息。
func ResolveAPIKeyRole(presented string) (Role, bool) {
	if presented == "" {
		return "", false
	}
	// 主 key:恒定时间比较。
	if serverAPIKey != "" && subtle.ConstantTimeCompare([]byte(presented), []byte(serverAPIKey)) == 1 {
		return ParseRole(GetAPIKeyRole()), true
	}
	scopedMu.RLock()
	defer scopedMu.RUnlock()
	for k, role := range scopedKeys {
		if subtle.ConstantTimeCompare([]byte(presented), []byte(k)) == 1 {
			return role, true
		}
	}
	return "", false
}

// ScopedAPIKeyCount 已注册的附加 key 数量(状态接口展示用)。
func ScopedAPIKeyCount() int {
	scopedMu.RLock()
	defer scopedMu.RUnlock()
	return len(scopedKeys)
}

// aiMCPKey 外部 coding agent 用的 MCP 专用 key。
var aiMCPKey string

// SetAIMCPAPIKey 设置/清除外部 agent 的 MCP 专用 key。
//
// **角色固定为 readonly** —— 这是治理决定,不是可调项:
// MCP 路径上危险工具**不弹确认**(只过角色闸门),
// 给外部 agent 一个可写的 key,等于把「无需确认的写操作」交出去。
// 外部 agent 要写操作 ⇒ 走 git-ferry 内置助手的确认链路。
//
// 重复设置会替换旧 key(旧 key 立即失效),不会留下孤儿凭据。
func SetAIMCPAPIKey(key string) {
	scopedMu.Lock()
	defer scopedMu.Unlock()
	if aiMCPKey != "" {
		delete(scopedKeys, aiMCPKey)
		aiMCPKey = ""
	}
	if key == "" {
		return
	}
	aiMCPKey = key
	scopedKeys[key] = RoleReadonly
}
