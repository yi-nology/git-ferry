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
	Enabled            bool    `json:"enabled"`
	BaseURL            string  `json:"base_url"`
	Model              string  `json:"model"`
	Temperature        float64 `json:"temperature"`
	MaxTokens          int     `json:"max_tokens"`
	TimeoutSeconds     int     `json:"timeout_seconds"`
	MaxConcurrentChats int     `json:"max_concurrent_chats"`
	// APIKey 明文仅存于本文件;读接口永远只回 has_api_key / masked。
	APIKey    string    `json:"api_key,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// SettingsView 读接口视图(不泄密钥)。
type SettingsView struct {
	Enabled            bool    `json:"enabled"`
	BaseURL            string  `json:"base_url"`
	Model              string  `json:"model"`
	Temperature        float64 `json:"temperature"`
	MaxTokens          int     `json:"max_tokens"`
	TimeoutSeconds     int     `json:"timeout_seconds"`
	MaxConcurrentChats int     `json:"max_concurrent_chats"`
	HasAPIKey          bool    `json:"has_api_key"`
	APIKeyMasked       string  `json:"api_key_masked"`
	UpdatedAt          string  `json:"updated_at,omitempty"`
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
func (s *SettingsStore) Save(in Settings) (Settings, error) {
	in.BaseURL = strings.TrimSpace(in.BaseURL)
	in.Model = strings.TrimSpace(in.Model)
	if in.Enabled {
		if in.BaseURL == "" {
			return Settings{}, fmt.Errorf("base_url 不能为空")
		}
		if in.Model == "" {
			return Settings{}, fmt.Errorf("model 不能为空")
		}
		// 密钥可沿用已存值:允许只改模型/地址不重传 key
		s.mu.RLock()
		prevKey := s.cur.APIKey
		s.mu.RUnlock()
		if in.APIKey == "" {
			in.APIKey = prevKey
		}
		if in.APIKey == "" {
			return Settings{}, fmt.Errorf("api_key 不能为空(或保留已保存的密钥)")
		}
	}
	if in.Temperature <= 0 {
		in.Temperature = 0.3
	}
	if in.MaxTokens <= 0 {
		in.MaxTokens = 2048
	}
	if in.TimeoutSeconds <= 0 {
		in.TimeoutSeconds = 60
	}
	if in.MaxConcurrentChats <= 0 {
		in.MaxConcurrentChats = 4
	}
	in.UpdatedAt = time.Now()

	s.mu.Lock()
	s.cur = in
	s.mu.Unlock()

	if err := s.persist(in); err != nil {
		return Settings{}, err
	}
	return in, nil
}

func (s *SettingsStore) persist(st Settings) error {
	if s.path == "" {
		return fmt.Errorf("ai settings path empty")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("mkdir ai settings: %w", err)
	}
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
func View(st Settings) SettingsView {
	v := SettingsView{
		Enabled:            st.Enabled,
		BaseURL:            st.BaseURL,
		Model:              st.Model,
		Temperature:        st.Temperature,
		MaxTokens:          st.MaxTokens,
		TimeoutSeconds:     st.TimeoutSeconds,
		MaxConcurrentChats: st.MaxConcurrentChats,
		HasAPIKey:          st.APIKey != "",
		APIKeyMasked:       MaskKey(st.APIKey),
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
func (st Settings) ToConfig() *Config {
	cfg := &Config{
		Enabled:            st.Enabled,
		BaseURL:            st.BaseURL,
		Model:              st.Model,
		Temperature:        st.Temperature,
		MaxTokens:          st.MaxTokens,
		TimeoutSeconds:     st.TimeoutSeconds,
		MaxConcurrentChats: st.MaxConcurrentChats,
	}
	if cfg.Temperature <= 0 {
		cfg.Temperature = 0.3
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 2048
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 60
	}
	if cfg.MaxConcurrentChats <= 0 {
		cfg.MaxConcurrentChats = 4
	}
	return cfg
}

// MergeSettings yaml 默认值 + 设置文件覆盖(设置文件为准,空则回退 yaml)。
func MergeSettings(yamlCfg Config, st Settings) Settings {
	out := st
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
	if out.APIKey == "" {
		out.APIKey = APIKeyFromEnv()
	}
	return out
}
