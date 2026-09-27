package corebridge

import (
	"testing"

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
