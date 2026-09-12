package model

import (
	"context"
	"fmt"
	"os"

	"github.com/sethvargo/go-envconfig"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Redis    RedisConfig    `yaml:"redis"`
	Git      GitConfig      `yaml:"git"`
	Sync     SyncConfig     `yaml:"sync"`
	Webhook  WebhookConfig  `yaml:"webhook"`
	Log      LogConfig      `yaml:"log"`
}

type ServerConfig struct {
	Host   string `yaml:"host" env:"GIT_SYNC_SERVER_HOST"`
	Port   int    `yaml:"port" env:"GIT_SYNC_SERVER_PORT"`
	Mode   string `yaml:"mode" env:"GIT_SYNC_SERVER_MODE"`
	APIKey string `yaml:"api_key" env:"GIT_SYNC_SERVER_API_KEY"`
}

type DatabaseConfig struct {
	Driver         string `yaml:"driver" env:"GIT_SYNC_DB_DRIVER"`
	DSN            string `yaml:"dsn" env:"GIT_SYNC_DB_DSN"`
	MaxIdleConns   int    `yaml:"max_idle_conns" env:"GIT_SYNC_DB_MAX_IDLE_CONNS"`
	MaxOpenConns   int    `yaml:"max_open_conns" env:"GIT_SYNC_DB_MAX_OPEN_CONNS"`
	ConnMaxLifeSec int    `yaml:"conn_max_life_sec" env:"GIT_SYNC_DB_CONN_MAX_LIFE_SEC"` // 连接最大存活秒数,0 用默认 5 分钟
	ConnMaxIdleSec int    `yaml:"conn_max_idle_sec" env:"GIT_SYNC_DB_CONN_MAX_IDLE_SEC"` // 空闲连接最大存活秒数,0 用默认 2 分钟
}

type RedisConfig struct {
	Addr            string `yaml:"addr" env:"GIT_SYNC_REDIS_ADDR"`
	Password        string `yaml:"password" env:"GIT_SYNC_REDIS_PASSWORD"`
	DB              int    `yaml:"db" env:"GIT_SYNC_REDIS_DB"`
	PoolSize        int    `yaml:"pool_size" env:"GIT_SYNC_REDIS_POOL_SIZE"`           // 连接池大小,0 用 go-redis 默认(10*GOMAXPROCS)
	MinIdleConns    int    `yaml:"min_idle_conns" env:"GIT_SYNC_REDIS_MIN_IDLE_CONNS"` // 最小空闲连接数,0 不预热
	DialTimeoutSec  int    `yaml:"dial_timeout_sec" env:"GIT_SYNC_REDIS_DIAL_TIMEOUT_SEC"`  // 建连超时秒数,0 不设超时
	ReadTimeoutSec  int    `yaml:"read_timeout_sec" env:"GIT_SYNC_REDIS_READ_TIMEOUT_SEC"`  // 读超时秒数,0 不设超时
	WriteTimeoutSec int    `yaml:"write_timeout_sec" env:"GIT_SYNC_REDIS_WRITE_TIMEOUT_SEC"` // 写超时秒数,0 不设超时
}

type GitConfig struct {
	Backend string `yaml:"backend" env:"GIT_SYNC_GIT_BACKEND"`
	TempDir string `yaml:"temp_dir" env:"GIT_SYNC_GIT_TEMP_DIR"`
}

type SyncConfig struct {
	MaxConcurrent  int `yaml:"max_concurrent" env:"GIT_SYNC_MAX_CONCURRENT"`
	DefaultTimeout int `yaml:"default_timeout" env:"GIT_SYNC_DEFAULT_TIMEOUT"`
	RetryCount     int `yaml:"retry_count" env:"GIT_SYNC_RETRY_COUNT"`
}

type WebhookConfig struct {
	RateLimit   int `yaml:"rate_limit" env:"GIT_SYNC_WEBHOOK_RATE_LIMIT"`
	MaxBodySize int `yaml:"max_body_size" env:"GIT_SYNC_WEBHOOK_MAX_BODY_SIZE"`
}

type LogConfig struct {
	Level  string `yaml:"level" env:"GIT_SYNC_LOG_LEVEL"`
	Format string `yaml:"format" env:"GIT_SYNC_LOG_FORMAT"`
}

// LoadConfig 从指定路径加载配置文件。
// path 参数由调用方控制（通常是启动参数或环境变量），不存在用户注入风险。
// 环境变量会覆盖 YAML 文件中的同名配置（env 优先级更高）。
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // 配置文件路径由程序启动参数决定，非用户可控输入
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// 环境变量覆盖 YAML 配置值（env 优先级高于 YAML）。
	// 使用 ProcessWith 配合 DefaultOverwrite=true 确保已由 YAML 设定的字段也能被环境变量覆盖。
	ctx := context.Background()
	if err := envconfig.ProcessWith(ctx, &envconfig.Config{
		Target:           &cfg,
		Lookuper:         envconfig.OsLookuper(),
		DefaultOverwrite: true,
	}); err != nil {
		return nil, fmt.Errorf("processing environment variables: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Validate 校验配置并填充默认值。
// 注意：环境变量覆盖发生在 Validate 之前，因此此处的默认值逻辑仅在
// YAML 和环境变量均未提供对应字段时才生效。
func (c *Config) Validate() error {
	if c.Server.Host == "" {
		c.Server.Host = DefaultHost
	}
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		c.Server.Port = DefaultPort
	}
	// 拒绝已知的测试默认 API Key,防止裸部署
	if c.Server.APIKey == "test-api-key-123" {
		return fmt.Errorf("refusing to start with default test API key; set a real api_key in config")
	}
	if c.Database.Driver == "" {
		return fmt.Errorf("database driver is required")
	}
	if c.Database.DSN == "" {
		return fmt.Errorf("database dsn is required")
	}
	if c.Database.Driver != DriverMySQL && c.Database.Driver != DriverSQLite {
		return fmt.Errorf("unsupported database driver: %s", c.Database.Driver)
	}
	if c.Database.MaxIdleConns <= 0 {
		c.Database.MaxIdleConns = DefaultMaxIdleConns
	}
	if c.Database.MaxOpenConns <= 0 {
		c.Database.MaxOpenConns = DefaultMaxOpenConns
	}
	if c.Database.ConnMaxLifeSec <= 0 {
		c.Database.ConnMaxLifeSec = DefaultConnMaxLifeSec
	}
	if c.Database.ConnMaxIdleSec <= 0 {
		c.Database.ConnMaxIdleSec = DefaultConnMaxIdleSec
	}
	if c.Git.TempDir == "" {
		c.Git.TempDir = DefaultTempDir
	}
	if c.Sync.MaxConcurrent <= 0 {
		c.Sync.MaxConcurrent = DefaultMaxConcurrent
	}
	if c.Sync.DefaultTimeout <= 0 {
		c.Sync.DefaultTimeout = DefaultTimeout
	}
	if c.Sync.RetryCount <= 0 {
		c.Sync.RetryCount = DefaultRetryCount
	}
	if c.Webhook.RateLimit <= 0 {
		c.Webhook.RateLimit = DefaultWebhookRateLimit
	}
	if c.Webhook.MaxBodySize <= 0 {
		c.Webhook.MaxBodySize = DefaultMaxBodySize
	}
	// Redis 超时默认值:0 意味着无限阻塞,生产环境必须有超时
	if c.Redis.Addr != "" {
		if c.Redis.DialTimeoutSec <= 0 {
			c.Redis.DialTimeoutSec = DefaultRedisDialTimeout
		}
		if c.Redis.ReadTimeoutSec <= 0 {
			c.Redis.ReadTimeoutSec = DefaultRedisReadTimeout
		}
		if c.Redis.WriteTimeoutSec <= 0 {
			c.Redis.WriteTimeoutSec = DefaultRedisWriteTimeout
		}
	}
	// 数值上限:防止配置错误导致资源耗尽
	if c.Sync.MaxConcurrent > MaxConcurrent {
		c.Sync.MaxConcurrent = MaxConcurrent
	}
	if c.Database.MaxOpenConns > MaxDBOpenConns {
		c.Database.MaxOpenConns = MaxDBOpenConns
	}
	if c.Webhook.MaxBodySize > MaxWebhookBodySize {
		c.Webhook.MaxBodySize = MaxWebhookBodySize
	}
	if c.Webhook.RateLimit > MaxWebhookRateLimit {
		c.Webhook.RateLimit = MaxWebhookRateLimit
	}
	return nil
}
