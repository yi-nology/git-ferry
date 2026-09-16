package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	return p
}

func TestLoadConfig_AISectionMissing(t *testing.T) {
	p := writeTemp(t, "server:\n  port: 8890\n")
	cfg, err := LoadConfig(p)
	require.NoError(t, err) // ai 段缺失不报错,功能默认关闭
	assert.False(t, cfg.Enabled)
}

func TestLoadConfig_AISection(t *testing.T) {
	p := writeTemp(t, `
ai:
  enabled: true
  base_url: "http://127.0.0.1:8899/v1"
  model: "gpt-stub"
  temperature: 0.2
  max_tokens: 1024
  timeout_seconds: 30
  max_concurrent_chats: 2
`)
	cfg, err := LoadConfig(p)
	require.NoError(t, err)
	assert.True(t, cfg.Enabled)
	assert.Equal(t, "http://127.0.0.1:8899/v1", cfg.BaseURL)
	assert.Equal(t, "gpt-stub", cfg.Model)
	assert.Equal(t, 0.2, cfg.Temperature)
	assert.Equal(t, 1024, cfg.MaxTokens)
	assert.Equal(t, 30, cfg.TimeoutSeconds)
	assert.Equal(t, 2, cfg.MaxConcurrentChats)
}

func TestValidate_Defaults(t *testing.T) {
	cfg := &Config{Enabled: true, BaseURL: "http://x/v1", Model: "m"}
	require.NoError(t, cfg.Validate("key"))
	assert.Equal(t, 0.3, cfg.Temperature)
	assert.Equal(t, 2048, cfg.MaxTokens)
	assert.Equal(t, 60, cfg.TimeoutSeconds)
	assert.Equal(t, 4, cfg.MaxConcurrentChats)
}

func TestValidate_MissingFields(t *testing.T) {
	base := &Config{Enabled: true}
	assert.ErrorContains(t, base.Validate("key"), "base_url")

	cfg2 := &Config{Enabled: true, BaseURL: "http://x/v1"}
	assert.ErrorContains(t, cfg2.Validate("key"), "model")

	cfg3 := &Config{Enabled: true, BaseURL: "http://x/v1", Model: "m"}
	assert.ErrorContains(t, cfg3.Validate(""), "GIT_SYNC_AI_API_KEY")
}

func TestValidate_Disabled(t *testing.T) {
	cfg := &Config{}
	assert.NoError(t, cfg.Validate("")) // 未启用时允许全空
}
