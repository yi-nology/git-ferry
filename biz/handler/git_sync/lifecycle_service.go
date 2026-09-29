package git_sync

import (
	"context"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/biz/model/ops"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// AutoDiscover POST /api/v1/ops/auto-discover
// 扫描平台仓库找本地未登记项;import_new=true 时自动导入。
func AutoDiscover(ctx context.Context, c *app.RequestContext) {
	var req ops.AutoDiscoverReq
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
		ExcludeArchived: optBoolDefault(req.ExcludeArchived),
		ExcludeForks:    optBoolDefault(req.ExcludeForks),
		MinStars:        int(req.MinStars),
		IncludeLanguage: req.IncludeLanguage,
	}
	rep, err := svc.AutoDiscover(ctx, req.PlatformKey, corebridge.AutoDiscoverOptions{
		ImportNew: optBool(req.ImportNew),
		Filter:    filter,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	action := "auto_discover_scan"
	if optBool(req.ImportNew) {
		action = "auto_discover_import"
	}
	recordAudit(ctx, c, action, "repo", req.PlatformKey,
		"发现新仓库 "+strconv.Itoa(len(rep.NewRepos)))
	response.Success(c, rep)
}

// DetectDrift POST /api/v1/ops/drift
// 比对本地 workdir 与目标远端分支 tip,发现静默漂移。
func DetectDrift(ctx context.Context, c *app.RequestContext) {
	var req ops.DetectDriftReq
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

// CleanupBackups POST /api/v1/ops/backup-cleanup
// 按 retention 天数清理;legal_hold 时拒绝。
func CleanupBackups(ctx context.Context, c *app.RequestContext) {
	var req ops.CleanupBackupsReq
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
