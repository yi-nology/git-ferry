package corebridge

import (
	synccore "github.com/yi-nology/git-sync-core"
	"github.com/yi-nology/git-sync-core/dao"
	"github.com/yi-nology/git-sync-core/model"
	coreservice "github.com/yi-nology/git-sync-core/service"
)

// ===== 入口 =====

type (
	Service        = synccore.Service
	Config         = synccore.Config
	WebhookPayload = synccore.WebhookPayload
)

func LoadConfig(path string) (*Config, error)  { return synccore.LoadConfig(path) }
func NewService(cfg *Config) (*Service, error) { return synccore.NewService(cfg) }

// ===== 领域模型（handler DTO 转换 / 审计等） =====

type (
	Repo               = model.Repo
	Platform           = model.Platform
	SyncTask           = model.SyncTask
	SyncRun            = model.SyncRun
	SyncRunStep        = model.SyncRunStep
	WebhookRule        = model.WebhookRule
	WebhookEvent       = model.WebhookEvent
	OperationLog       = model.OperationLog
	CreateRepoRequest  = model.CreateRepoRequest
	UpdateRepoRequest  = model.UpdateRepoRequest
	CreateTaskRequest  = model.CreateTaskRequest
	UpdateTaskRequest  = model.UpdateTaskRequest
	PreviewSyncRequest = model.PreviewSyncRequest
	PreviewSyncResult  = model.PreviewSyncResult
	CreateRuleRequest  = model.CreateRuleRequest
	UpdateRuleRequest  = model.UpdateRuleRequest
)

const StatusSuccess = model.StatusSuccess

// 触发来源常量(手动触发路径使用)
const TriggerManual = model.TriggerManual

// 平台常量 / 工具（平台管理 handler 使用）
const (
	PlatformTypeCustom   = model.PlatformTypeCustom
	PlatformStatusActive = model.PlatformStatusActive
	PlatformStatusError  = model.PlatformStatusError
)

// ValidPlatformType 检查平台类型是否合法(SDK 注册表 + 扩展白名单)。
func ValidPlatformType(t string) bool { return model.ValidPlatformType(t) }

func GetAPIURL(platformType, instanceURL string) string {
	return model.GetAPIURL(platformType, instanceURL)
}

// ===== DAO 过滤器 / 分页 =====

type (
	RepoFilter         = dao.RepoFilter
	OperationLogFilter = dao.OperationLogFilter
)

func DefaultPagination(offset, limit int) dao.Pagination {
	return dao.DefaultPagination(offset, limit)
}

// ===== 业务错误 =====

var (
	ErrRepoNotFound = coreservice.ErrRepoNotFound
	ErrTaskNotFound = coreservice.ErrTaskNotFound
	ErrRuleNotFound = coreservice.ErrRuleNotFound
)
