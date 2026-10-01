package git_sync

// gitServe 配置由 main 启动时注入（git_serve 段）。
var gitServeEnabled bool
var gitServeBasePath string
var gitServePublic bool

// SetGitServe 启用/配置只读 Git Smart HTTP。
func SetGitServe(enabled bool, basePath string, publicRead bool) {
	gitServeEnabled = enabled
	gitServeBasePath = basePath
	gitServePublic = publicRead
}

// GitServeEnabled 返回是否启用。
func GitServeEnabled() bool { return gitServeEnabled }

// GitServeBasePath 返回 bare 仓库根目录。
func GitServeBasePath() string { return gitServeBasePath }

// GitServePublic 是否免鉴权。
func GitServePublic() bool { return gitServePublic }
