package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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

// TestSettings_Save_profile用acpx加ask要拒 某个 profile 用 acpx + ask 时起不来;
// 放过它 ⇒ 存下一份「部分后端起不来」的配置,界面显示已保存、实际行为不符。
func TestSettings_Save_profile用acpx加ask要拒(t *testing.T) {
	store, err := OpenSettings(filepath.Join(t.TempDir(), "ai-settings.json"))
	require.NoError(t, err)

	_, err = store.Save(&Settings{
		Enabled: true,
		Backend: BackendACPX,
		External: ExternalSettings{
			Agent: "mcode",
			Agents: []ExternalAgentConfig{
				{Name: "codex", Transport: BackendACPX, Agent: "codex", Permission: "ask"},
			},
		},
	})
	assert.ErrorContains(t, err, "codex", "错误信息要指出是哪个 profile")

	// 换成 stdio 就没问题
	_, err = store.Save(&Settings{
		Enabled: true,
		Backend: BackendACPX,
		External: ExternalSettings{
			Agent: "mcode",
			Agents: []ExternalAgentConfig{
				{Name: "codex", Transport: BackendStdio, Command: "codex-acp", Permission: "ask"},
			},
		},
	})
	assert.NoError(t, err, "stdio 传输支持逐次确认")
}

// TestMergeSettings_不漏字段 用反射逐字段核对:yaml 的 External 里每个字段
// 都必须在合并结果里体现(空值回退规则除外)。
//
// **为什么要有这个测试**:"逐字段列举"的合并必然还会再漏 ——
// 这次新增具名 profile 列表(Agents/Current)时就漏了,
// 表现为「yaml 里配了 profile,启动时却当没配」。
// 反射比对能让下一次新增字段**当场**被拦住。
func TestMergeSettings_不漏字段(t *testing.T) {
	yamlCfg := &Config{
		Backend: BackendACPX,
		External: ExternalConfig{ //nolint:gosec // G101:字段名含 Key,此处是环境变量名,不是凭据
			Agent:          "yaml-agent",
			Command:        "yaml-cmd",
			Args:           []string{"-x"},
			Cwd:            "/yaml-ws",
			TimeoutSeconds: 111,
			Permission:     "approve_reads",
			MCPEnabled:     true,
			MCPURL:         "http://yaml/mcp",
			MCPAPIKeyEnv:   "YAML_KEY_ENV",
			MCPServers:     []MCPServerConfig{{Name: "yaml-srv"}},
			Agents:         []ExternalAgentConfig{{Name: "yaml-profile"}},
			Current:        "yaml-profile",
		},
	}
	// 设置文件为空(首次未写)⇒ 合并结果应完整等于 yaml 的 External。
	got := MergeSettings(yamlCfg, &Settings{})

	rv := reflect.ValueOf(got.External)
	ry := reflect.ValueOf(yamlCfg.External)
	ty := rv.Type()
	for i := 0; i < ty.NumField(); i++ {
		name := ty.Field(i).Name
		gv := rv.Field(i).Interface()
		yv := ry.Field(i).Interface()
		if gv == nil || reflect.ValueOf(gv).IsZero() {
			continue
		}
		assert.Equal(t, yv, gv, "字段 %s 没从 yaml 合并过来(逐字段列举必漏,用反射兜住)", name)
	}
	assert.Equal(t, "yaml-profile", got.External.Current)
	assert.Len(t, got.External.Agents, 1)
}

// TestSettings_JSON键名必须是snake_case 设置文件与 API 回显都走 JSON ⇒
// 字段名必须与前端约定的一致(agent/agents/mcp_servers…)。
//
// **踩过的坑**:ExternalConfig 只有 yaml tag,被当成设置文件结构复用后,
// JSON 回显的是 Go 字段名(`Agent`/`Agents`)⇒ 前端表单读不到 ⇒ 界面全空。
// 这个测试把「键名」钉死。
func TestSettings_JSON键名必须是snake_case(t *testing.T) {
	st := Settings{
		Enabled: true,
		Backend: BackendACPX,
		External: ExternalConfig{
			Agent:        "mcode",
			Cwd:          "/ws",
			Permission:   "approve_reads",
			MCPEnabled:   true,
			MCPURL:       "http://x/mcp",
			MCPAPIKeyEnv: "ENV_X",
			MCPServers:   []MCPServerConfig{{Name: "s1"}},
			Agents:       []ExternalAgentConfig{{Name: "p1", Transport: "stdio"}},
			Current:      "p1",
		},
	}
	b, err := json.Marshal(st) //nolint:gosec // G117:测的是 JSON 键名,结构体里没有真实凭据
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	ext, ok := m["external"].(map[string]any)
	require.True(t, ok, "external 段应存在")
	for _, key := range []string{"agent", "cwd", "permission", "mcp_enabled", "mcp_url",
		"mcp_api_key_env", "mcp_servers", "agents", "current"} {
		_, has := ext[key]
		assert.True(t, has, "JSON 键 %q 缺失(前端按 snake_case 读)", key)
	}
	// 也不能冒出 Go 字段名
	for _, bad := range []string{"Agent", "Cwd", "MCPEnabled", "Agents"} {
		_, has := ext[bad]
		assert.False(t, has, "JSON 里不该出现 Go 字段名 %q", bad)
	}
}
