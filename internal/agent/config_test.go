package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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

// TestValidate_按后端分支 选了哪个后端,就按那个后端的要求校验。
//
// 这不是「给调用方兜底默认值」—— 后端是显式选择,不是可推断的缺省。
func TestValidate_按后端分支(t *testing.T) {
	// internal:要 base_url + model + apiKey
	ic := &Config{Enabled: true, Backend: BackendInternal}
	assert.ErrorContains(t, ic.Validate("key"), "base_url")
	ic.BaseURL = "http://x/v1"
	assert.ErrorContains(t, ic.Validate("key"), "model")
	ic.Model = "m"
	assert.ErrorContains(t, ic.Validate(""), "GIT_SYNC_AI_API_KEY")
	require.NoError(t, ic.Validate("key"))

	// acpx:要 agent 或 command;**不**要求 base_url/model/apiKey
	ac := &Config{Enabled: true, Backend: BackendACPX}
	assert.ErrorContains(t, ac.Validate(""), "external.agent")
	ac.External.Agent = "mcode"
	require.NoError(t, ac.Validate(""), "外部后端不该要求模型 API Key")

	// stdio:要 command
	sc := &Config{Enabled: true, Backend: BackendStdio}
	assert.ErrorContains(t, sc.Validate(""), "external.command")
	sc.External.Command = "mcode acp"
	require.NoError(t, sc.Validate(""))

	// 未知后端直接报错
	uc := &Config{Enabled: true, Backend: "carrier-pigeon"}
	assert.ErrorContains(t, uc.Validate("key"), "ai.backend")
}

// TestValidate_默认后端是internal 未指定后端时按内置助手处理。
func TestValidate_默认后端是internal(t *testing.T) {
	c := &Config{Enabled: true, BaseURL: "http://x/v1", Model: "m"}
	require.NoError(t, c.Validate("key"))
	assert.Equal(t, BackendInternal, c.Backend)
}

// TestSettings_后端与外部配置往返 设置文件(yaml 覆盖层)要能完整承载后端配置。
func TestSettings_后端与外部配置往返(t *testing.T) {
	st := Settings{
		Enabled: true,
		Backend: BackendACPX,
		External: ExternalSettings{ //nolint:gosec // G101:字段名含 Key,此处是环境变量名,不是凭据
			Agent:          "mcode",
			Cwd:            "var/ai-workspace",
			Permission:     "ask",
			TimeoutSeconds: 600,
			MCPEnabled:     true,
			MCPAPIKeyEnv:   "GITFERRY_AI_MCP_KEY_ENV",
		},
	}
	cfg := st.ToConfig()
	assert.Equal(t, BackendACPX, cfg.Backend)
	assert.Equal(t, "mcode", cfg.External.Agent)
	assert.True(t, cfg.External.MCPEnabled)

	// （视图不回传密钥;环境变量名要带出来）
	v := View(&st)
	assert.Equal(t, BackendACPX, v.Backend)
	assert.Equal(t, "mcode", v.External.Agent)
}

// TestMergeSettings_布尔的yaml值要生效 布尔没法区分「未设置」与「显式 false」,
// 所以在设置文件还没写过之前,yaml 的布尔值说了算。
// 少了这一句 ⇒ yaml 里的 mcp_enabled: true 永远不生效。
func TestMergeSettings_布尔的yaml值要生效(t *testing.T) {
	yamlCfg := &Config{
		Enabled: true,
		Backend: BackendStdio,
		External: ExternalConfig{
			Command:    "mcode acp",
			MCPEnabled: true,
		},
	}
	// 设置文件为空(首次未写)
	got := MergeSettings(yamlCfg, &Settings{})
	assert.True(t, got.External.MCPEnabled, "yaml 的 mcp_enabled 应生效")
	assert.Equal(t, "mcode acp", got.External.Command)

	// 设置文件写过了 ⇒ 它的值说了算(哪怕是 false)
	got2 := MergeSettings(yamlCfg, &Settings{
		UpdatedAt: time.Now(),
		External:  ExternalSettings{MCPEnabled: false},
	})
	assert.False(t, got2.External.MCPEnabled, "设置文件写过之后,它的值优先")
}

// TestSettings_Save_拒绝存不可用的组合 存下一份「后端起不来」的配置,
// 界面上显示已保存、实际 AI 是关的 —— 又是「配置与实际行为不符」。
func TestSettings_Save_拒绝存不可用的组合(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenSettings(filepath.Join(dir, "ai-settings.json"))
	require.NoError(t, err)

	// acpx + ask:非交互通道弹不出授权卡 ⇒ 拒
	_, err = store.Save(&Settings{
		Enabled: true,
		Backend: BackendACPX,
		External: ExternalSettings{
			Agent:      "mcode",
			Permission: "ask",
		},
	})
	assert.ErrorContains(t, err, "非交互")

	// 存量配置没被动过
	assert.False(t, store.Get().Enabled)
}
