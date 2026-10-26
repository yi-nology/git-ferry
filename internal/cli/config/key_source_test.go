package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeCfg 写一个临时配置文件并把 GITFERRY_CONFIG 指过去。
func writeCfg(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	t.Setenv("GITFERRY_CONFIG", p)
}

// TestEffective_KeySource_环境变量 环境变量优先,且来源要标出来。
func TestEffective_KeySource_环境变量(t *testing.T) {
	writeCfg(t, "base_url: http://x\napi_key: from-file\n")
	t.Setenv("GITFERRY_TOKEN", "from-env")

	cfg, err := Effective()
	require.NoError(t, err)
	assert.Equal(t, "from-env", cfg.APIKey)
	assert.Equal(t, "env:GITFERRY_TOKEN", cfg.KeySource)
}

// TestEffective_KeySource_文件 配置文件里有 key ⇒ 来源是 file。
func TestEffective_KeySource_文件(t *testing.T) {
	writeCfg(t, "base_url: http://x\napi_key: from-file\n")
	t.Setenv("GITFERRY_TOKEN", "")
	t.Setenv("GITFERRY_API_KEY", "")

	cfg, err := Effective()
	require.NoError(t, err)
	assert.Equal(t, "from-file", cfg.APIKey)
	assert.Equal(t, "file", cfg.KeySource)
}

// TestEffective_KeySource_都没有 两处都没有 ⇒ none,且**不**该谎称读不到。
func TestEffective_KeySource_都没有(t *testing.T) {
	writeCfg(t, "base_url: http://x\n")
	t.Setenv("GITFERRY_TOKEN", "")
	t.Setenv("GITFERRY_API_KEY", "")

	cfg, err := Effective()
	require.NoError(t, err)
	assert.Empty(t, cfg.APIKey)
	assert.Equal(t, "none", cfg.KeySource)
}

// TestEffective_KeySource_不落盘 KeySource / KeychainErr 是运行时信息,
// 不能被写进配置文件(yaml:"-" )。
func TestEffective_KeySource_不落盘(t *testing.T) {
	writeCfg(t, "base_url: http://x\n")
	t.Setenv("GITFERRY_TOKEN", "secret-env")
	t.Setenv("GITFERRY_API_KEY", "")

	cfg, err := Effective()
	require.NoError(t, err)
	require.Equal(t, "env:GITFERRY_TOKEN", cfg.KeySource)

	path, err := Path()
	require.NoError(t, err)
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(b), "key_source")
	assert.NotContains(t, string(b), "keychain_error")
}
