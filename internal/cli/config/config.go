package config

import (
	"errors"
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

	// KeySource 密钥来源：env:GITFERRY_TOKEN / env:GITFERRY_API_KEY /
	// file / keychain / none。**不落盘**（yaml:"-"）。
	//
	// 为什么要有它：密钥存在 macOS Keychain 时，配置文件里的 api_key 是空的
	// ⇒ 「本机到底有没有密钥」这件事**从文件上看不出来**
	// ⇒ 出问题时只能看到服务端的 401「unauthorized」，无从判断是
	// "密钥不对"还是"这台机器上根本没拿到密钥"。
	KeySource string `yaml:"-"`

	// KeychainErr 从 Keychain 取密钥时的失败原因（条目不存在除外）。
	// 只用于给出**准确**的提示，不改变行为。
	KeychainErr string `yaml:"-"`
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
	switch {
	case os.Getenv(envToken) != "":
		cfg.APIKey = os.Getenv(envToken)
		cfg.KeySource = "env:" + envToken
	case os.Getenv(envAPIKey) != "":
		cfg.APIKey = os.Getenv(envAPIKey)
		cfg.KeySource = "env:" + envAPIKey
	case cfg.APIKey != "":
		cfg.KeySource = "file"
	default:
		// Keychain 里**可能**存着（配置文件里是空的）⇒ 取不到时要分清
		// "条目不存在"与"读不到"，两者对用户的建议完全不同。
		v, kerr := loadSecret("api_key")
		switch {
		case kerr == nil && v != "":
			cfg.APIKey = v
			cfg.KeySource = "keychain"
		case kerr != nil && !errors.Is(kerr, errSecretNotFound):
			cfg.KeySource = "none"
			cfg.KeychainErr = kerr.Error()
		default:
			cfg.KeySource = "none"
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
