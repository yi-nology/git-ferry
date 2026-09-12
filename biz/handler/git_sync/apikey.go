package git_sync

// 壳层 API Key（公网默认鉴权）。不进 git-sync-core。
// 启动时由 main 经 corebridge.LoadShellConfig 注入。

var serverAPIKey string

// SetAPIKey 由壳层 main 在路由注册前调用。
func SetAPIKey(key string) {
	serverAPIKey = key
}

// GetAPIKey 返回当前配置的 API Key；空串表示未配置。
func GetAPIKey() string {
	return serverAPIKey
}
