package git_sync

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// OpsTrends GET /api/v1/ops/trends?days=30
// 同步成功率/耗时时间序列（供前端小图）。聚合在 core Service.OpsTrends。
func OpsTrends(ctx context.Context, c *app.RequestContext) {
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	days := 30
	if v := parseIntQuery(c, "days"); v > 0 && v <= 365 {
		days = v
	}
	res, err := svc.OpsTrends(ctx, days)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, map[string]any{
		"days":         res.Days,
		"items":        res.Items,
		"total":        res.Total,
		"success":      res.Success,
		"failed":       res.Failed,
		"success_rate": res.SuccessRate,
	})
}

// parseIntQuery 只接受纯数字，非法/超界返回 0。
func parseIntQuery(c *app.RequestContext, name string) int {
	v := c.Query(name)
	if v == "" {
		return 0
	}
	n := 0
	for _, ch := range v {
		if ch < '0' || ch > '9' {
			return 0
		}
		n = n*10 + int(ch-'0')
		if n > 100000 {
			return 0
		}
	}
	return n
}
