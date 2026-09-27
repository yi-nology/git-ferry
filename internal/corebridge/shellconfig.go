package corebridge

import (
	"os"

	"gopkg.in/yaml.v3"

	"github.com/yi-nology/git-ferry/internal/notify"
)

// ShellConfig 引擎配置 + 壳层登录/通知配置（不进 core）。
type ShellConfig struct {
	*Config
	// APIKey 公网壳默认 X-API-Key；内网壳可忽略，改用网关/SSO。
	APIKey string
	// Notify 多通道通知（ntfy/gotify/heartbeat）。
	Notify *notify.Config
	// RunWatch 运行观察与失败自动补偿设置。
	// 独立 struct 而非 runwatch.Config:corebridge 不依赖 runwatch,避免 import 环。
	RunWatch *RunWatchSettings
}

// RunWatchSettings 对应 yaml runwatch 段,main 启动时转成 runwatch.Config。
type RunWatchSettings struct {
	IntervalSeconds int `yaml:"interval_seconds"`
	HistoryLimit    int `yaml:"history_limit"`
	Retry           struct {
		MaxAutoRetries  int `yaml:"max_auto_retries"`
		CooldownMinutes int `yaml:"cooldown_minutes"`
	} `yaml:"retry"`
}

// shellOverlay 壳层专有 YAML 段。
type shellOverlay struct {
	Server struct {
		APIKey string `yaml:"api_key"`
	} `yaml:"server"`
	Notify   *notify.Config    `yaml:"notify"`
	RunWatch *RunWatchSettings `yaml:"runwatch"`
}

// LoadShellConfig 加载 core 配置，并叠加壳层 server.api_key / notify / runwatch。
//
//	INTEGRATION: yaml server.api_key 或环境变量 GIT_SYNC_SERVER_API_KEY
func LoadShellConfig(path string) (*ShellConfig, error) {
	cfg, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}
	overlay, err := readShellOverlay(path)
	if err != nil {
		return nil, err
	}
	apiKey := os.Getenv("GIT_SYNC_SERVER_API_KEY")
	if apiKey == "" {
		apiKey = overlay.Server.APIKey
	}
	// 拒绝已知弱默认值，防止裸部署(示例/测试密钥不得用于生产)
	switch apiKey {
	case "test-api-key-123", "dev-local-key", "change-me-to-a-strong-random-key", "change-me":
		apiKey = ""
	}
	return &ShellConfig{
		Config:   cfg,
		APIKey:   apiKey,
		Notify:   overlay.Notify,
		RunWatch: overlay.RunWatch,
	}, nil
}

func readShellOverlay(path string) (*shellOverlay, error) {
	data, err := os.ReadFile(path) //nolint:gosec // 启动配置路径由部署方控制
	if err != nil {
		return nil, err
	}
	var overlay shellOverlay
	if err := yaml.Unmarshal(data, &overlay); err != nil {
		return nil, err
	}
	return &overlay, nil
}
