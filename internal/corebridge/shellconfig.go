package corebridge

import (
	"os"

	"gopkg.in/yaml.v3"
)

// ShellConfig 引擎配置 + 壳层登录配置（API Key 不进 core）。
type ShellConfig struct {
	*Config
	// APIKey 公网壳默认 X-API-Key；内网壳可忽略，改用网关/SSO。
	APIKey string
}

// LoadShellConfig 加载 core 配置，并叠加壳层 server.api_key（env 优先）。
//
//	INTEGRATION: yaml server.api_key 或环境变量 GIT_SYNC_SERVER_API_KEY
func LoadShellConfig(path string) (*ShellConfig, error) {
	cfg, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}
	apiKey := os.Getenv("GIT_SYNC_SERVER_API_KEY")
	if apiKey == "" {
		apiKey, err = readAPIKeyFromYAML(path)
		if err != nil {
			return nil, err
		}
	}
	// 拒绝已知测试默认值，防止裸部署
	if apiKey == "test-api-key-123" {
		apiKey = ""
	}
	return &ShellConfig{Config: cfg, APIKey: apiKey}, nil
}

func readAPIKeyFromYAML(path string) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // 启动配置路径由部署方控制
	if err != nil {
		return "", err
	}
	var overlay struct {
		Server struct {
			APIKey string `yaml:"api_key"`
		} `yaml:"server"`
	}
	if err := yaml.Unmarshal(data, &overlay); err != nil {
		return "", err
	}
	return overlay.Server.APIKey, nil
}
