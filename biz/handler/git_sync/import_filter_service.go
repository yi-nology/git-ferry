package git_sync

import (
	"context"
	"github.com/yi-nology/git-ferry/biz/model/ops"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/oekit/pkg/ptrx"
	"github.com/oekit/pkg/slicex"
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
		IncludeGlobs:    splitCommaList(ptrx.Deref(req.IncludeGlobs, "")),
		ExcludeGlobs:    splitCommaList(ptrx.Deref(req.ExcludeGlobs, "")),
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

// splitCommaList 按逗号切分,逐项去首尾空白并丢弃空项。
// 空串(全空白)直接返回 nil——多留一个空项会让空 glob 匹配所有东西。
// 切分后无有效项时同样返回 nil(空列表按"未提供"处理)。
func splitCommaList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := slicex.Filter(stringx.SplitByComma(s), func(p string) bool {
		return strings.TrimSpace(p) != ""
	})
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	if len(parts) == 0 {
		return nil
	}
	return parts
}
