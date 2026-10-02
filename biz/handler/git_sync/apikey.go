package git_sync

import "os"

// 壳层 API Key（公网默认鉴权）。不进 git-ferry-core。
// 启动时由 main 经 corebridge.LoadShellConfig 注入。

var serverAPIKey string
var serverAPIKeyRole string

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
