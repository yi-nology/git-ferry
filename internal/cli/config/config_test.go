package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GITFERRY_CONFIG", filepath.Join(dir, "config.yaml"))

	cfg := &Config{BaseURL: "http://example:1234", APIKey: "secret", Format: "yaml"}
	require.NoError(t, Save(cfg))

	got, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "http://example:1234", got.BaseURL)
	assert.Equal(t, "secret", got.APIKey)
	assert.Equal(t, "yaml", got.Format)

	// 权限 0600
	info, err := os.Stat(filepath.Join(dir, "config.yaml"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestEffective_EnvOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GITFERRY_CONFIG", filepath.Join(dir, "config.yaml"))
	require.NoError(t, Save(&Config{BaseURL: "http://file:1", APIKey: "from-file", Format: "json"}))

	t.Setenv("GITFERRY_BASE_URL", "http://env:2")
	t.Setenv("GITFERRY_TOKEN", "from-env")
	t.Setenv("GITFERRY_FORMAT", "table")

	got, err := Effective()
	require.NoError(t, err)
	assert.Equal(t, "http://env:2", got.BaseURL)
	assert.Equal(t, "from-env", got.APIKey)
	assert.Equal(t, "table", got.Format)
}

func TestLoad_MissingFile(t *testing.T) {
	t.Setenv("GITFERRY_CONFIG", filepath.Join(t.TempDir(), "nope.yaml"))
	got, err := Load()
	require.NoError(t, err)
	assert.Equal(t, DefaultBaseURL, got.BaseURL)
}

func TestSaveAPIKey_Clear(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	t.Setenv("GITFERRY_CONFIG", path)

	cfg := &Config{BaseURL: DefaultBaseURL, APIKey: "k1"}
	// SaveAPIKey: macOS 可能进 keychain（文件不落密钥）；其他平台写文件
	_ = SaveAPIKey(cfg)

	cfg2 := &Config{BaseURL: DefaultBaseURL}
	require.NoError(t, ClearAPIKey(cfg2))
	got, err := Load()
	require.NoError(t, err)
	assert.Empty(t, got.APIKey)
}
