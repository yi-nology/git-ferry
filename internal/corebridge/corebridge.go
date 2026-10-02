package corebridge

import (
	synccore "github.com/yi-nology/git-ferry-core"
	"github.com/yi-nology/git-ferry-core/dao"
	"github.com/yi-nology/git-ferry-core/executor"
	"github.com/yi-nology/git-ferry-core/model"
	"github.com/yi-nology/git-ferry-core/pkg/deploykey"
	coreservice "github.com/yi-nology/git-ferry-core/service"
	"github.com/yi-nology/go-git-platform/pkg/credential"
	sdkprov "github.com/yi-nology/go-git-platform/provider"
	"gorm.io/gorm"
)

// ===== 入口 =====

type (
	Service        = synccore.Service
	Config         = synccore.Config
	WebhookPayload = synccore.WebhookPayload
)

func LoadConfig(path string) (*Config, error) { return synccore.LoadConfig(path) }
func NewService(cfg *Config, opts ...Option) (*Service, error) {
	return synccore.NewService(cfg, opts...)
}

// ===== 装配期依赖注入（见 core service/options.go） =====

type Option = coreservice.Option

func WithDB(db *gorm.DB) Option { return coreservice.WithDB(db) }

func WithProviderHooks(h *sdkprov.Hooks) Option { return coreservice.WithProviderHooks(h) }

func WithForcePushApprover(a executor.ForcePushApprover) Option {
	return coreservice.WithForcePushApprover(a)
}

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
	PlatformTypeGitHub   = model.PlatformTypeGitHub
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
	RepoFilter           = dao.RepoFilter
	RepoImportFilter     = coreservice.RepoImportFilter
	OperationLogFilter   = dao.OperationLogFilter
	AutoDiscoverOptions  = coreservice.AutoDiscoverOptions
	DiscoveryReport      = coreservice.DiscoveryReport
	DriftReport          = coreservice.DriftReport
	AuditChainResult     = coreservice.AuditChainResult
	DriftItem            = coreservice.DriftItem
	ForcePushPolicy      = coreservice.ForcePushPolicy
	RPOReport            = coreservice.RPOReport
	RPOMetric            = coreservice.RPOMetric
	BackupManifest       = coreservice.BackupManifest
	ManifestVerifyResult = coreservice.ManifestVerifyResult
	DrillReport          = coreservice.DrillReport
)

func DefaultPagination(offset, limit int) dao.Pagination {
	return dao.DefaultPagination(offset, limit)
}

// ===== 业务错误 =====

var (
	ErrRepoNotFound           = coreservice.ErrRepoNotFound
	ErrTaskNotFound           = coreservice.ErrTaskNotFound
	ErrRuleNotFound           = coreservice.ErrRuleNotFound
	ErrPlatformNotFound       = coreservice.ErrPlatformNotFound
	ErrTargetPlatformNotFound = coreservice.ErrTargetPlatformNotFound
	ErrMetadataValidation     = coreservice.ErrMetadataValidation
)

// 错误到传输层的分类契约（壳 response.FromError 据此选状态码）。
type ErrorClass = coreservice.ErrorClass

const (
	ErrorClassNotFound    = coreservice.ErrorClassNotFound
	ErrorClassConflict    = coreservice.ErrorClassConflict
	ErrorClassValidation  = coreservice.ErrorClassValidation
	ErrorClassAuth        = coreservice.ErrorClassAuth
	ErrorClassRateLimited = coreservice.ErrorClassRateLimited
	ErrorClassUnavailable = coreservice.ErrorClassUnavailable
	ErrorClassInternal    = coreservice.ErrorClassInternal
)

func Classify(err error) ErrorClass { return coreservice.Classify(err) }

// ===== 元数据备份/回灌引擎（core service 下沉，壳 handler 经此取类型） =====

type (
	MetadataBackupOptions = coreservice.MetadataBackupOptions
	MetadataBackupResult  = coreservice.MetadataBackupResult
	MetadataSnapshot      = coreservice.MetadataSnapshot
	MetadataBackupInfo    = coreservice.MetadataBackupInfo
	RestoreRequest        = coreservice.RestoreRequest
	RestoreResult         = coreservice.RestoreResult
	RestoreKindStat       = coreservice.RestoreKindStat
	VerifyReport          = coreservice.VerifyReport
)

// ===== org 映射（org_import / org_mirror 共用 core 唯一实现） =====

type (
	OrgMapStrategy = coreservice.OrgMapStrategy
	OrgMapOptions  = coreservice.OrgMapOptions
	OrgMapResult   = coreservice.OrgMapResult
)

const (
	OrgMapPreserve = coreservice.OrgMapPreserve
	OrgMapSingle   = coreservice.OrgMapSingle
	OrgMapFlat     = coreservice.OrgMapFlat
	OrgMapMixed    = coreservice.OrgMapMixed
)

func ParseOrgMapStrategy(s string) (OrgMapStrategy, error) { return coreservice.ParseStrategy(s) }

func ValidOrgMapStrategy(s string) bool { return coreservice.ValidOrgMapStrategy(s) }

// ResolveOrgTarget 目标 owner/repo 映射（OrgMapOptions 较大，按指针传）。
func ResolveOrgTarget(sourceOwner, sourceRepo string, o *OrgMapOptions) (OrgMapResult, error) {
	return coreservice.ResolveOrgTarget(sourceOwner, sourceRepo, *o)
}

func IsPersonalOwner(owner, targetUser string, orgOwners map[string]bool) bool {
	return coreservice.IsPersonalOwner(owner, targetUser, orgOwners)
}

// ===== 运行完成事件 / 失败自动补偿（core events.go + retry_policy.go） =====

type (
	RunEvent        = coreservice.RunEvent
	AutoRetryPolicy = coreservice.AutoRetryPolicy
	RetryDecision   = coreservice.Decision
	RetryTracker    = coreservice.RetryTracker
)

func NewRetryTracker(maxKeep int) *RetryTracker { return coreservice.NewRetryTracker(maxKeep) }

// ===== 部署密钥（core pkg/deploykey） =====

// GenerateDeployKey 生成镜像部署用 Ed25519 密钥对（私钥仅返回一次，不落库）。
func GenerateDeployKey(comment string) (*credential.DeployKey, error) {
	return deploykey.Generate(comment)
}

// ===== 组织导入 / 组织镜像编排（core orgbulk_service） =====

type (
	PublicOrgImportRequest = coreservice.PublicOrgImportRequest
	PublicOrgImportResult  = coreservice.PublicOrgImportResult
	OrgRepoItem            = coreservice.OrgRepoItem
	BulkMirrorRequest      = coreservice.BulkMirrorRequest
	BulkMirrorResult       = coreservice.BulkMirrorResult
	BulkMirrorItem         = coreservice.BulkMirrorItem
)
