package git_sync

import (
	"context"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// OpsTrends GET /api/v1/ops/trends?days=30
// 同步成功率/耗时时间序列（供前端小图）。
func OpsTrends(ctx context.Context, c *app.RequestContext) {
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	days := 30
	if v := parseIntQuery(c, "days"); v > 0 && v <= 365 {
		days = v
	}
	type dayAgg struct {
		Date    string  `json:"date"`
		Total   int     `json:"total"`
		Success int     `json:"success"`
		Failed  int     `json:"failed"`
		AvgMs   float64 `json:"avg_duration_ms"`
	}
	// 简化：扫描最近 N 天全部 history（上限 2000 条）按日聚合
	runs, _, err := svc.ListHistory(ctx, "", 0, 2000)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	byDay := map[string]*dayAgg{}
	order := []string{}
	for _, r := range runs {
		if r.StartTime.Before(cutoff) {
			continue
		}
		d := r.StartTime.Format("2006-01-02")
		agg, exists := byDay[d]
		if !exists {
			agg = &dayAgg{Date: d}
			byDay[d] = agg
			order = append(order, d)
		}
		agg.Total++
		switch r.Status {
		case "success":
			agg.Success++
		case "failed":
			agg.Failed++
		}
		agg.AvgMs += float64(r.DurationMs)
	}
	items := make([]*dayAgg, 0, len(order))
	for _, d := range order {
		agg := byDay[d]
		if agg.Total > 0 {
			agg.AvgMs /= float64(agg.Total)
		}
		items = append(items, agg)
	}
	// 总计
	total, success, failed := 0, 0, 0
	for _, a := range items {
		total += a.Total
		success += a.Success
		failed += a.Failed
	}
	rate := 0.0
	if total > 0 {
		rate = float64(success) / float64(total) * 100
	}
	response.Success(c, map[string]any{
		"days":         days,
		"items":        items,
		"total":        total,
		"success":      success,
		"failed":       failed,
		"success_rate": rate,
	})
}

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
