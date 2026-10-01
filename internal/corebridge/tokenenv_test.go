package corebridge

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeEnvKey(t *testing.T) {
	assert.Equal(t, "MY_GITHUB", sanitizeEnvKey("my-github"))
	assert.Equal(t, "A_B_C", sanitizeEnvKey("a.b/c"))
}

func TestResolveTokenFromEnv(t *testing.T) {
	t.Setenv("GIT_SYNC_TOKEN_MY_GITHUB", "from-env")
	assert.Equal(t, "from-env", ResolveTokenFromEnv("my-github", "from-body"))
	assert.Equal(t, "fallback", ResolveTokenFromEnv("other", "fallback"))
	assert.Equal(t, "x", ResolveTokenFromEnv("", "x"))
}

func TestResolveTokenFromCmd(t *testing.T) {
	// CMD 优先于 ENV
	t.Setenv("GIT_SYNC_TOKEN_CMD_MY_GITHUB", "echo 'from-cmd'")
	t.Setenv("GIT_SYNC_TOKEN_MY_GITHUB", "from-env")
	assert.Equal(t, "from-cmd", ResolveTokenFromEnv("my-github", "from-body"))

	// CMD 失败 → 回退 ENV
	t.Setenv("GIT_SYNC_TOKEN_CMD_MY_GITHUB", "false")
	assert.Equal(t, "from-env", ResolveTokenFromEnv("my-github", "from-body"))

	// 多行只取首行
	t.Setenv("GIT_SYNC_TOKEN_CMD_MY_GITHUB", "printf 'line1\\nline2\\n'")
	assert.Equal(t, "line1", ResolveTokenFromEnv("my-github", "from-body"))
}

func TestRunTokenCmdTimeout(t *testing.T) {
	// 超时返回空(1ms 不够跑 sleep)
	got := runTokenCmd("sleep 1", 1*time.Millisecond)
	assert.Equal(t, "", got)
}
