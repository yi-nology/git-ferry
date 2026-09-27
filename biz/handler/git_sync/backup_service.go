package git_sync

import (
	"context"
	"path/filepath"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// ListBundlesReq 列出冷备。
type ListBundlesReq struct {
	TaskKey string `json:"task_key" form:"task_key" query:"task_key"`
}

// ListBundles GET /api/v1/ops/bundles
func ListBundles(ctx context.Context, c *app.RequestContext) {
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	list, err := svc.ListBundles(c.Query("task_key"))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, map[string]any{
		"backup_dir": svc.BackupDir(),
		"items":      list,
		"total":      len(list),
	})
}

// VerifyBundleReq 校验 bundle。
type VerifyBundleReq struct {
	Name string `json:"name" form:"name" query:"name"`
}

// VerifyBundle GET /api/v1/ops/bundles/verify?name=
func VerifyBundle(ctx context.Context, c *app.RequestContext) {
	name := c.Query("name")
	if name == "" {
		response.BadRequest(c, "name is required")
		return
	}
	if filepath.Base(name) != name {
		response.BadRequest(c, "invalid bundle name")
		return
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	info, err := svc.VerifyBundle(ctx, name)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, info)
}

// RestoreBundleReq 从 bundle 恢复。
type RestoreBundleReq struct {
	Name    string `json:"name" form:"name" query:"name"`
	DestDir string `json:"dest_dir" form:"dest_dir" query:"dest_dir"`
}

// RestoreBundle POST /api/v1/ops/bundles/restore
func RestoreBundle(ctx context.Context, c *app.RequestContext) {
	var req RestoreBundleReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Name == "" || req.DestDir == "" {
		response.BadRequest(c, "name and dest_dir are required")
		return
	}
	if filepath.Base(req.Name) != req.Name {
		response.BadRequest(c, "invalid bundle name")
		return
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	if err := svc.RestoreBundle(ctx, req.Name, req.DestDir); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	recordAudit(ctx, c, "restore_bundle", "backup", req.Name, "从冷备恢复到 "+req.DestDir)
	response.Success(c, map[string]any{
		"success": true,
		"dest":    req.DestDir,
		"name":    req.Name,
	})
}
