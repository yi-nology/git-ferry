package git_sync

import (
	"context"
	"github.com/yi-nology/git-ferry/biz/model/ops"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
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
		ExcludeArchived: optBool(req.ExcludeArchived),
		ExcludeForks:    optBool(req.ExcludeForks),
		MinStars:        int(req.MinStars),
		IncludeLanguage: optStr(req.IncludeLanguage),
		IncludeGlobs:    splitCSV(optStr(req.IncludeGlobs)),
		ExcludeGlobs:    splitCSV(optStr(req.ExcludeGlobs)),
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

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
