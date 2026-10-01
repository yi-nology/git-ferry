package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config 是 gitferry CLI 的本地配置（~/.config/gitferry/config.yaml）。
type Config struct {
	BaseURL string `yaml:"base_url"`
	APIKey  string `yaml:"api_key"`
	Format  string `yaml:"format"`
}

const (
	DefaultBaseURL = "http://127.0.0.1:8890"
	envBaseURL     = "GITFERRY_BASE_URL"
	envToken       = "GITFERRY_TOKEN"
	envAPIKey      = "GITFERRY_API_KEY" //nolint:gosec // 环境变量名，非密钥字面量
	envFormat      = "GITFERRY_FORMAT"
)

// Path 返回配置文件路径。
func Path() (string, error) {
	if x := os.Getenv("GITFERRY_CONFIG"); x != "" {
		return x, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "gitferry", "config.yaml"), nil
}

// Load 读取配置文件（不存在返回空配置）。
func Load() (*Config, error) {
	cfg := &Config{BaseURL: DefaultBaseURL, Format: "json"}
	path, err := Path()
	if err != nil {
		return cfg, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	if err := yaml.Unmarshal(b, cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

// Save 写入配置文件（0600）。
func Save(cfg *Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	//nolint:gosec // G117: 配置文件本就存 api_key（0600），macOS 上优先写入 Keychain 后文件留空
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// Effective 合并配置文件与环境变量（环境变量优先；api_key 再尝试 OS Keychain）。
func Effective() (*Config, error) {
	cfg, err := Load()
	if err != nil {
		return nil, err
	}
	if v := os.Getenv(envBaseURL); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv(envToken); v != "" {
		cfg.APIKey = v
	} else if v := os.Getenv(envAPIKey); v != "" {
		cfg.APIKey = v
	} else if cfg.APIKey == "" {
		if v := loadSecret("api_key"); v != "" {
			cfg.APIKey = v
		}
	}
	if v := os.Getenv(envFormat); v != "" {
		cfg.Format = v
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	return cfg, nil
}

// SaveAPIKey 优先写入 OS Keychain，失败则并入配置文件（0600）。
func SaveAPIKey(cfg *Config) error {
	if saveSecret("api_key", cfg.APIKey) {
		// Keychain 成功：文件里不留明文
		cfgCopy := *cfg
		cfgCopy.APIKey = ""
		return Save(&cfgCopy)
	}
	return Save(cfg)
}

// ClearAPIKey 清除 Keychain 与文件中的密钥。
func ClearAPIKey(cfg *Config) error {
	deleteSecret("api_key")
	cfg.APIKey = ""
	return Save(cfg)
}
