package corebridge

import (
	"errors"
	"os"

	"github.com/yi-nology/git-ferry-core/model"
	"gopkg.in/yaml.v3"

	"github.com/yi-nology/git-ferry/internal/notify"
)

// ShellConfig 引擎配置 + 壳层登录/通知配置（不进 core）。
type ShellConfig struct {
	*Config
	// APIKey 公网壳默认 X-API-Key；内网壳可忽略，改用网关/SSO。
	APIKey string
	// APIKeyRole 共享 API Key 的 RBAC 角色(admin/operator/readonly),默认 admin。
	APIKeyRole string
	// OIDC 可选 JWT Bearer 鉴权(HS256)。
	OIDC *OIDCSettings
	// Notify 多通道通知（ntfy/gotify/heartbeat）。
	Notify *notify.Config
	// RunWatch 运行观察与失败自动补偿设置。
	// 独立 struct 而非 runwatch.Config:corebridge 不依赖 runwatch,避免 import 环。
	RunWatch *RunWatchSettings
	// GitServe 只读 Git Smart HTTP（局域网/灾备 clone）。
	GitServe *GitServeSettings
}

// GitServeSettings 对应 yaml git_serve 段。
type GitServeSettings struct {
	Enabled bool `yaml:"enabled"`
	// BasePath bare 仓库根目录；默认 <backup_dir>/git-serve
	BasePath string `yaml:"base_path"`
	// PublicRead true=无需鉴权（仅内网）；默认 false
	PublicRead bool `yaml:"public_read"`
}

// OIDCSettings 对应 yaml auth.oidc 段。
type OIDCSettings struct {
	Enabled     bool   `yaml:"enabled"`
	Secret      string `yaml:"secret"`
	Issuer      string `yaml:"issuer"`
	Audience    string `yaml:"audience"`
	RoleClaim   string `yaml:"role_claim"`
	UserClaim   string `yaml:"user_claim"`
	DefaultRole string `yaml:"default_role"`
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
		APIKey     string `yaml:"api_key"`
		APIKeyRole string `yaml:"api_key_role"`
	} `yaml:"server"`
	Auth struct {
		OIDC *OIDCSettings `yaml:"oidc"`
	} `yaml:"auth"`
	Notify   *notify.Config    `yaml:"notify"`
	RunWatch *RunWatchSettings `yaml:"runwatch"`
	GitServe *GitServeSettings `yaml:"git_serve"`
}

// LoadShellConfig 加载 core 配置，并叠加壳层 server.api_key / auth.oidc / notify / runwatch。
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
	apiKeyRole := os.Getenv("GIT_SYNC_API_KEY_ROLE")
	if apiKeyRole == "" {
		apiKeyRole = overlay.Server.APIKeyRole
	}
	// 轻量校验：对齐 conf/config.schema.json 的关键枚举/范围
	if cfg != nil {
		if err := validateSyncConfig(&cfg.Sync); err != nil {
			return nil, err
		}
	}
	return &ShellConfig{
		Config:     cfg,
		APIKey:     apiKey,
		APIKeyRole: apiKeyRole,
		OIDC:       overlay.Auth.OIDC,
		Notify:     overlay.Notify,
		RunWatch:   overlay.RunWatch,
		GitServe:   overlay.GitServe,
	}, nil
}

// validateSyncConfig 启动时校验 sync 段（schema 子集）。
func validateSyncConfig(s *model.SyncConfig) error {
	if s == nil {
		return nil
	}
	if s.BackupFormat != "" && s.BackupFormat != "bundle" && s.BackupFormat != "zip" {
		return errors.New("sync.backup_format must be bundle|zip, got " + s.BackupFormat)
	}
	if s.BackupKeep < 0 {
		return errors.New("sync.backup_keep must be >= 0")
	}
	if s.MaxConcurrent < 0 {
		return errors.New("sync.max_concurrent must be >= 0")
	}
	return nil
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
