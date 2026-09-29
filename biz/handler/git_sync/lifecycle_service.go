package git_sync

import (
	"context"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// AutoDiscoverReq 自动发现请求。
type AutoDiscoverReq struct {
	PlatformKey     string `json:"platform_key" form:"platform_key" query:"platform_key"`
	ImportNew       bool   `json:"import_new" form:"import_new" query:"import_new"`
	ExcludeArchived *bool  `json:"exclude_archived" form:"exclude_archived"`
	ExcludeForks    *bool  `json:"exclude_forks" form:"exclude_forks"`
	MinStars        int    `json:"min_stars" form:"min_stars"`
	IncludeLanguage string `json:"include_language" form:"include_language"`
}

// AutoDiscover POST /api/v1/ops/auto-discover
// 扫描平台仓库找本地未登记项;import_new=true 时自动导入。
func AutoDiscover(ctx context.Context, c *app.RequestContext) {
	var req AutoDiscoverReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.PlatformKey == "" {
		response.BadRequest(c, "platform_key is required")
		return
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	filter := &corebridge.RepoImportFilter{
		ExcludeArchived: req.ExcludeArchived == nil || *req.ExcludeArchived,
		ExcludeForks:    req.ExcludeForks == nil || *req.ExcludeForks,
		MinStars:        req.MinStars,
		IncludeLanguage: req.IncludeLanguage,
	}
	rep, err := svc.AutoDiscover(ctx, req.PlatformKey, corebridge.AutoDiscoverOptions{
		ImportNew: req.ImportNew,
		Filter:    filter,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	action := "auto_discover_scan"
	if req.ImportNew {
		action = "auto_discover_import"
	}
	recordAudit(ctx, c, action, "repo", req.PlatformKey,
		"发现新仓库 "+strconv.Itoa(len(rep.NewRepos)))
	response.Success(c, rep)
}

// DetectDriftReq 漂移检测请求。
type DetectDriftReq struct {
	TaskKeys []string `json:"task_keys" form:"task_keys" query:"task_keys"`
}

// DetectDrift POST /api/v1/ops/drift
// 比对本地 workdir 与目标远端分支 tip,发现静默漂移。
func DetectDrift(ctx context.Context, c *app.RequestContext) {
	var req DetectDriftReq
	_ = c.BindAndValidate(&req)
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	rep, err := svc.DetectDrift(ctx, req.TaskKeys)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, rep)
}

// CleanupBackupsReq 冷备清理请求。
type CleanupBackupsReq struct {
	Confirm string `json:"confirm" form:"confirm" query:"confirm"`
}

// CleanupBackups POST /api/v1/ops/backup-cleanup
// 按 retention 天数清理;legal_hold 时拒绝。
func CleanupBackups(ctx context.Context, c *app.RequestContext) {
	var req CleanupBackupsReq
	_ = c.BindAndValidate(&req)
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	if req.Confirm != "yes" {
		response.BadRequest(c, "confirm=yes required")
		return
	}
	removed, err := svc.CleanupExpiredBackups()
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	recordAudit(ctx, c, "backup_cleanup", "backup", "", "清理过期冷备 "+strconv.Itoa(removed))
	response.Success(c, map[string]any{
		"removed":    removed,
		"legal_hold": svc.LegalHold(),
	})
}
