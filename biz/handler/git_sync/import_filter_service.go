package git_sync

import (
	"context"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// SyncPlatformFilteredReq 按过滤条件导入平台仓库。
type SyncPlatformFilteredReq struct {
	Key             string `json:"key" form:"key" query:"key"`
	ExcludeArchived bool   `json:"exclude_archived" form:"exclude_archived" query:"exclude_archived"`
	ExcludeForks    bool   `json:"exclude_forks" form:"exclude_forks" query:"exclude_forks"`
	MinStars        int    `json:"min_stars" form:"min_stars" query:"min_stars"`
	IncludeLanguage string `json:"include_language" form:"include_language" query:"include_language"`
	IncludeGlobs    string `json:"include_globs" form:"include_globs" query:"include_globs"` // 逗号分隔
	ExcludeGlobs    string `json:"exclude_globs" form:"exclude_globs" query:"exclude_globs"`
}

// SyncPlatformFiltered POST /api/v1/ops/sync-platform
// 借鉴 gickup filter:导入时排除 archived/fork、按 star/语言/glob 裁剪。
func SyncPlatformFiltered(ctx context.Context, c *app.RequestContext) {
	var req SyncPlatformFilteredReq
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
		ExcludeArchived: req.ExcludeArchived,
		ExcludeForks:    req.ExcludeForks,
		MinStars:        req.MinStars,
		IncludeLanguage: req.IncludeLanguage,
		IncludeGlobs:    splitCSV(req.IncludeGlobs),
		ExcludeGlobs:    splitCSV(req.ExcludeGlobs),
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
