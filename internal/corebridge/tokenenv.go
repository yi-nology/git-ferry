package corebridge

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ResolveTokenFromEnv 用环境变量覆盖平台访问令牌。
//
//	INTEGRATION: GIT_SYNC_TOKEN_<PLATFORM_KEY 大写并把 - 换成 _>
//	例:platform key=my-github → GIT_SYNC_TOKEN_MY_GITHUB
//
// 密钥来源优先级(借鉴 ghorg token_cmd / 1Password 集成):
//  1. GIT_SYNC_TOKEN_CMD_<KEY> —— 执行命令,stdout 第一行作为 token
//     例:GIT_SYNC_TOKEN_CMD_MY_GITHUB='op read op://infra/gh/token'
//  2. GIT_SYNC_TOKEN_<KEY> —— 直接注入
//  3. 返回 current(配置/DB 中的值)
//
// 设计意图:密钥不进配置文件/DB 明文,由部署方以 env/secrets manager 注入。
// 命令超时 5s,失败则回退到 env/原值。
func ResolveTokenFromEnv(platformKey, current string) string {
	if platformKey == "" {
		return current
	}
	key := sanitizeEnvKey(platformKey)

	if cmdStr := os.Getenv("GIT_SYNC_TOKEN_CMD_" + key); cmdStr != "" {
		if tok := runTokenCmd(cmdStr, 5*time.Second); tok != "" {
			return tok
		}
	}
	if v := os.Getenv("GIT_SYNC_TOKEN_" + key); v != "" {
		return v
	}
	return current
}

// runTokenCmd 执行密钥获取命令,取 stdout 首行去空白;超时返回空串。
//
// 用 exec.CommandContext:超时会杀掉子进程。迁移前是 goroutine + time.After,
// 超时后只是放弃等待,子进程仍在后台跑(泄漏,且可能一直挂着)。
func runTokenCmd(cmdStr string, timeout time.Duration) string {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// #nosec G204 G702 —— 命令来自部署方 GIT_SYNC_TOKEN_CMD_* 环境变量，
	// 与密钥注入同等信任级（ghorg token_cmd 同模型），非用户请求参数
	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr) //nolint:gosec // 见上
	cmd.Env = os.Environ()
	// 超时/失败时 b 为空 → 解析出空串 → 调用方回退到 env/原值,与迁移前一致
	b, _ := cmd.Output()

	s := strings.TrimSpace(string(b))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	return s
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
