package model

// Run status constants
const (
	StatusRunning = "running"
	StatusSuccess = "success"
	StatusFailed  = "failed"
)

// Webhook event status constants
const (
	StatusReceived   = "received"
	StatusProcessing = "processing"
	StatusProcessed  = "processed"
)

// Repository status constants
const (
	RepoStatusActive = "active"
)

// Trigger source constants
const (
	TriggerManual  = "manual"
	TriggerCron    = "cron"
	TriggerWebhook = "webhook"
)

// Webhook rule action constants
const (
	ActionSync = "sync"
)

// Database driver constants
const (
	DriverMySQL  = "mysql"
	DriverSQLite = "sqlite"
)

// Default configuration values
const (
	DefaultHost          = "0.0.0.0"
	DefaultPort          = 8890
	DefaultMaxIdleConns  = 10
	DefaultMaxOpenConns  = 100
	DefaultMaxConcurrent = 5
	DefaultTimeout       = 300
	DefaultRetryCount    = 3
	DefaultTempDir       = "/tmp/git-sync"

	DefaultConnMaxLifeSec = 300 // 5 分钟
	DefaultConnMaxIdleSec = 120 // 2 分钟
	DefaultWebhookRateLimit = 10
	DefaultMaxBodySize    = 10 << 20 // 10MB
	DefaultRedisDialTimeout = 5
	DefaultRedisReadTimeout = 3
	DefaultRedisWriteTimeout = 3

	// 上限值
	MaxConcurrent     = 100
	MaxDBOpenConns    = 200
	MaxWebhookBodySize = 100 << 20 // 100MB
	MaxWebhookRateLimit = 10000

	// Push retry 退避因子
	RetryBackoffMs = 500
)

// Step name constants
const (
	StepClone        = "clone"
	StepFetch        = "fetch"
	StepCheckout     = "checkout"
	StepEnsureRemote = "ensure_remote"
	StepPush         = "push"
)

// Error type constants
const (
	ErrorAuth    = "auth"
	ErrorNetwork = "network"
	ErrorConfig  = "config"
	ErrorGit     = "git"
	ErrorUnknown = "unknown"
)

// Table name constants
const (
	TableRepos            = "repos"
	TableSyncTasks        = "sync_tasks"
	TableSyncRuns         = "sync_runs"
	TableSyncRunSteps     = "sync_run_steps"
	TableWebhookRules     = "webhook_rules"
	TableWebhookRuleTasks = "webhook_rule_tasks"
	TableWebhookEvents    = "webhook_events"
	TableOperationLogs    = "operation_logs"
)

// Model default value constants
const (
	DefaultBranch   = "main"
	DefaultPlatform = "unknown"
	DefaultSyncMode = "single"
	DefaultEventType = "push"
	DefaultAction   = "sync"
)
