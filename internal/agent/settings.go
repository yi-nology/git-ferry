// Package agent — AI 运行时设置文件存储(data/ai-settings.json)。
// 与 conf/config.yaml 的 ai 段互补：yaml 为部署默认值,设置文件为界面改写后的覆盖层;
// API Key 只进设置文件(与「密钥不进 yaml」约定一致)。
package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Settings 前端可改的 AI 配置(持久化到 data/ai-settings.json)。
type Settings struct {
	Enabled bool `json:"enabled"`
	// Backend 执行后端:internal|acpx|stdio。
	Backend            string  `json:"backend"`
	BaseURL            string  `json:"base_url"`
	Model              string  `json:"model"`
	Temperature        float64 `json:"temperature"`
	MaxTokens          int     `json:"max_tokens"`
	TimeoutSeconds     int     `json:"timeout_seconds"`
	MaxConcurrentChats int     `json:"max_concurrent_chats"`
	// APIKey 明文仅存于本文件;读接口永远只回 has_api_key / masked。
	//nolint:gosec // G117:字段名 APIKey 命中密钥模式,此处确为密钥存储,不回传明文
	APIKey    string    `json:"api_key,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`

	// External 外部 coding agent 配置(backend=acpx|stdio 时生效)。
	External ExternalSettings `json:"external"`
}

// ExternalSettings 外部 coding agent 的界面可改配置。
//
// **不含密钥**:挂给 agent 的 MCP 专用 key 走环境变量(ExternalConfig.MCPAPIKeyEnv),
// 界面上只填「环境变量名」。
type ExternalSettings struct {
	Agent          string   `json:"agent"`
	Command        string   `json:"command"`
	Args           []string `json:"args"`
	Cwd            string   `json:"cwd"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	Permission     string   `json:"permission"`
	MCPEnabled     bool     `json:"mcp_enabled"`
	MCPURL         string   `json:"mcp_url"`
	MCPAPIKeyEnv   string   `json:"mcp_api_key_env"`
	// MCPServers 额外挂给外部 agent 的 MCP 服务器(可多个)。
	//
	// 与 MCPServerConfig 同结构(共用一套类型,免得两套平行定义漂移)。
	MCPServers []MCPServerConfig `json:"mcp_servers"`
}

// SettingsView 读接口视图(不泄密钥)。
type SettingsView struct {
	Enabled            bool    `json:"enabled"`
	Backend            string  `json:"backend"`
	BaseURL            string  `json:"base_url"`
	Model              string  `json:"model"`
	Temperature        float64 `json:"temperature"`
	MaxTokens          int     `json:"max_tokens"`
	TimeoutSeconds     int     `json:"timeout_seconds"`
	MaxConcurrentChats int     `json:"max_concurrent_chats"`
	HasAPIKey          bool    `json:"has_api_key"`
	APIKeyMasked       string  `json:"api_key_masked"`
	UpdatedAt          string  `json:"updated_at,omitempty"`

	// External 外部 coding agent 配置视图。
	External ExternalSettings `json:"external"`
}

// SettingsStore 文件型设置库(并发安全)。
type SettingsStore struct {
	mu   sync.RWMutex
	path string
	cur  Settings
}

// OpenSettings 打开设置库;不存在则返回空设置。
func OpenSettings(path string) (*SettingsStore, error) {
	s := &SettingsStore{path: path}
	data, err := os.ReadFile(path) //nolint:gosec // 数据目录由部署方控制
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("read ai settings: %w", err)
	}
	if err := json.Unmarshal(data, &s.cur); err != nil {
		return nil, fmt.Errorf("parse ai settings: %w", err)
	}
	return s, nil
}

// Get 返回当前设置快照。
func (s *SettingsStore) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur
}

// Save 校验后落盘并替换内存快照。
func (s *SettingsStore) Save(in *Settings) (*Settings, error) {
	in.BaseURL = strings.TrimSpace(in.BaseURL)
	in.Model = strings.TrimSpace(in.Model)
	in.External.Agent = strings.TrimSpace(in.External.Agent)
	in.External.Command = strings.TrimSpace(in.External.Command)
	in.External.Cwd = strings.TrimSpace(in.External.Cwd)
	in.External.MCPURL = strings.TrimSpace(in.External.MCPURL)
	in.External.MCPAPIKeyEnv = strings.TrimSpace(in.External.MCPAPIKeyEnv)
	if in.Backend == "" {
		in.Backend = BackendInternal
	}
	if in.Enabled {
		// **按后端分支校验**:选了哪个后端,就按那个后端的要求校验。
		switch in.Backend {
		case BackendInternal:
			if in.BaseURL == "" {
				return nil, fmt.Errorf("base_url 不能为空")
			}
			if in.Model == "" {
				return nil, fmt.Errorf("model 不能为空")
			}
			// 模型密钥只对**内置后端**有意义:外部后端根本不走
			// OpenAI 兼容端点,拿不到这个 key。放在分支外会让
			// 「用外部 coding agent」根本存不下去。
			// 密钥可沿用已存值:允许只改模型/地址不重传 key
			s.mu.RLock()
			prevKey := s.cur.APIKey
			s.mu.RUnlock()
			if in.APIKey == "" {
				in.APIKey = prevKey
			}
			if in.APIKey == "" {
				return nil, fmt.Errorf("api_key 不能为空(或保留已保存的密钥)")
			}
		case BackendACPX:
			if in.External.Agent == "" && in.External.Command == "" {
				return nil, fmt.Errorf("外部 agent 名为空(填 agent 如 mcode,或填自定义 command)")
			}
		case BackendStdio:
			if in.External.Command == "" {
				return nil, fmt.Errorf("外部 agent 命令为空(如 \"mcode acp\")")
			}
		default:
			return nil, fmt.Errorf("未知后端: %s(可选 internal|acpx|stdio)", in.Backend)
		}
	}
	in.applyDefaults()
	in.UpdatedAt = time.Now()

	s.mu.Lock()
	s.cur = *in
	s.mu.Unlock()

	if err := s.persist(in); err != nil {
		return nil, err
	}
	return in, nil
}

func (s *SettingsStore) persist(st *Settings) error {
	if s.path == "" {
		return fmt.Errorf("ai settings path empty")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return fmt.Errorf("mkdir ai settings: %w", err)
	}
	//nolint:gosec // G117:APIKey 仅序列化到本地 0600 设置文件,读接口不回传明文
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write ai settings: %w", err)
	}
	return os.Rename(tmp, s.path)
}

// View 生成脱敏视图。
func View(st *Settings) SettingsView {
	v := SettingsView{
		Enabled:            st.Enabled,
		Backend:            st.Backend,
		BaseURL:            st.BaseURL,
		Model:              st.Model,
		Temperature:        st.Temperature,
		MaxTokens:          st.MaxTokens,
		TimeoutSeconds:     st.TimeoutSeconds,
		MaxConcurrentChats: st.MaxConcurrentChats,
		HasAPIKey:          st.APIKey != "",
		APIKeyMasked:       MaskKey(st.APIKey),
		External:           st.External,
	}
	if !st.UpdatedAt.IsZero() {
		v.UpdatedAt = st.UpdatedAt.Format(time.RFC3339)
	}
	return v
}

// MaskKey 密钥脱敏:sk-xxxx…abcd。
func MaskKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

// ToConfig 转成运行时 agent.Config(补齐默认)。
func (st *Settings) ToConfig() *Config {
	cfg := &Config{
		Enabled:            st.Enabled,
		Backend:            st.Backend,
		BaseURL:            st.BaseURL,
		Model:              st.Model,
		Temperature:        st.Temperature,
		MaxTokens:          st.MaxTokens,
		TimeoutSeconds:     st.TimeoutSeconds,
		MaxConcurrentChats: st.MaxConcurrentChats,
		External: ExternalConfig{
			Agent:          st.External.Agent,
			Command:        st.External.Command,
			Args:           st.External.Args,
			Cwd:            st.External.Cwd,
			TimeoutSeconds: st.External.TimeoutSeconds,
			Permission:     st.External.Permission,
			MCPEnabled:     st.External.MCPEnabled,
			MCPURL:         st.External.MCPURL,
			MCPAPIKeyEnv:   st.External.MCPAPIKeyEnv,
			MCPServers:     st.External.MCPServers,
		},
	}
	cfg.applyDefaults()
	return cfg
}

// MergeSettings yaml 默认值 + 设置文件覆盖(设置文件为准,空则回退 yaml)。
func MergeSettings(yamlCfg *Config, st *Settings) Settings {
	out := *st
	if out.Backend == "" {
		out.Backend = yamlCfg.Backend
	}
	if out.External.Agent == "" {
		out.External.Agent = yamlCfg.External.Agent
	}
	if out.External.Command == "" {
		out.External.Command = yamlCfg.External.Command
	}
	if len(out.External.Args) == 0 {
		out.External.Args = yamlCfg.External.Args
	}
	if out.External.Cwd == "" {
		out.External.Cwd = yamlCfg.External.Cwd
	}
	if out.External.TimeoutSeconds == 0 {
		out.External.TimeoutSeconds = yamlCfg.External.TimeoutSeconds
	}
	if out.External.Permission == "" {
		out.External.Permission = yamlCfg.External.Permission
	}
	if out.External.MCPURL == "" {
		out.External.MCPURL = yamlCfg.External.MCPURL
	}
	if out.External.MCPAPIKeyEnv == "" {
		out.External.MCPAPIKeyEnv = yamlCfg.External.MCPAPIKeyEnv
	}
	if len(out.External.MCPServers) == 0 {
		out.External.MCPServers = yamlCfg.External.MCPServers
	}
	if out.BaseURL == "" {
		out.BaseURL = yamlCfg.BaseURL
	}
	if out.Model == "" {
		out.Model = yamlCfg.Model
	}
	if out.Temperature == 0 {
		out.Temperature = yamlCfg.Temperature
	}
	if out.MaxTokens == 0 {
		out.MaxTokens = yamlCfg.MaxTokens
	}
	if out.TimeoutSeconds == 0 {
		out.TimeoutSeconds = yamlCfg.TimeoutSeconds
	}
	if out.MaxConcurrentChats == 0 {
		out.MaxConcurrentChats = yamlCfg.MaxConcurrentChats
	}
	// 设置文件未启用时,尊重 yaml 的 enabled(首次未写文件前)
	if !st.Enabled && st.UpdatedAt.IsZero() {
		out.Enabled = yamlCfg.Enabled
	}
	// **布尔字段同理**:布尔没法区分「未设置」与「显式 false」,
	// 所以在设置文件还没写过之前,yaml 的布尔值说了算。
	// 少了这一句 ⇒ yaml 里的 mcp_enabled: true 永远不生效(被当成 false 覆盖掉)。
	if st.UpdatedAt.IsZero() {
		out.External.MCPEnabled = yamlCfg.External.MCPEnabled
	}
	if out.APIKey == "" {
		out.APIKey = APIKeyFromEnv()
	}
	return out
}
