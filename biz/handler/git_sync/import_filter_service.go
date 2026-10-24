package git_sync

import (
	"context"
	"github.com/yi-nology/git-ferry/biz/model/ops"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/oekit/pkg/ptrx"
	"github.com/oekit/pkg/stringx"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// SyncPlatformFiltered POST /api/v1/ops/sync-platform
// 借鉴 gickup filter:导入时排除 archived/fork、按 star/语言/glob 裁剪。
func SyncPlatformFiltered(ctx context.Context, c *app.RequestContext) {
	var req ops.SyncPlatformFilteredReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Key == "" {
		response.BadRequest(c, "key is required")
		return
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	filter := &corebridge.RepoImportFilter{
		ExcludeArchived: ptrx.Deref(req.ExcludeArchived, false),
		ExcludeForks:    ptrx.Deref(req.ExcludeForks, false),
		MinStars:        int(req.MinStars),
		IncludeLanguage: ptrx.Deref(req.IncludeLanguage, ""),
		IncludeGlobs:    stringx.SplitCSVNonEmpty(ptrx.Deref(req.IncludeGlobs, "")),
		ExcludeGlobs:    stringx.SplitCSVNonEmpty(ptrx.Deref(req.ExcludeGlobs, "")),
	}
	count, err := svc.SyncPlatformReposFiltered(ctx, req.Key, filter)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAudit(ctx, c, "sync_platform_filtered", "platform", req.Key,
		"过滤导入平台仓库")
	response.Success(c, map[string]any{
		"success":  true,
		"imported": count,
		"filter":   filter,
	})
}
