package corebridge

import (
	"os"
	"strings"
)

// ResolveTokenFromEnv 用环境变量覆盖平台访问令牌。
//
//	INTEGRATION: GIT_SYNC_TOKEN_<PLATFORM_KEY 大写并把 - 换成 _>
//	例:platform key=my-github → GIT_SYNC_TOKEN_MY_GITHUB
//
// 设计意图(借鉴 ghorg token_cmd):密钥不进配置文件/DB 明文,
// 由部署方以 env/secret manager 注入。命中则返回 env 值,否则返回原 token。
func ResolveTokenFromEnv(platformKey, current string) string {
	if platformKey == "" {
		return current
	}
	name := "GIT_SYNC_TOKEN_" + sanitizeEnvKey(platformKey)
	if v := os.Getenv(name); v != "" {
		return v
	}
	return current
}

func sanitizeEnvKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}
