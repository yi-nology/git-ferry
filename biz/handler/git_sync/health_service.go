package git_sync

import (
	"context"
	"time"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/yi-nology/git-ferry/biz/model/ops"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// ===== 健康评分 + 同步概览 =====
//
// 评分规则/维度在 core 的 health 包，取事实与打分在 Service.HealthSnapshot；
// 壳层只做入参绑定、可选的漂移检测触发与响应包装。

// HealthScore 按维度规则给任务打分(借鉴 OpenSSF Scorecards)。
// 维度：reliability(35) / freshness(20) / schedule(15) / safety(15) / completeness(15)
func HealthScore(ctx context.Context, c *app.RequestContext) {
	var req ops.HealthScoreReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	// with_drift=1 时做一次漂移检测并折入 safety 维度（网络开销，默认关闭）
	withDrift := c.Query("with_drift") == "1" || c.Query("with_drift") == "true"
	var driftByTask map[string]int
	if withDrift {
		if rep, derr := svc.DetectDrift(ctx, nil); derr == nil && rep != nil {
			driftByTask = map[string]int{}
			for _, it := range rep.Items {
				if it.Drifted {
					driftByTask[it.TaskKey]++
				}
			}
		}
	}

	snap, err := svc.HealthSnapshot(ctx, int(req.Limit), driftByTask)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, map[string]any{
		"items": snap.Items,
		"total": len(snap.Items),
		"summary": map[string]any{
			"levels":       snap.Summary.Levels,
			"below_silver": snap.Summary.BelowSilver,
			"top_actions":  snap.Summary.TopActions,
		},
		"attention":    snap.Summary.Attention,
		"generated_at": time.Now().Format(time.RFC3339),
	})
}

// SyncOverview 同步侧仪表盘指标。
func SyncOverview(ctx context.Context, c *app.RequestContext) {
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	taskStatus, err := svc.CountTasksByStatus()
	if err != nil {
		response.InternalError(c, "count tasks failed")
		return
	}
	repoCount, err := svc.CountRepos()
	if err != nil {
		response.InternalError(c, "count repos failed")
		return
	}
	failed, _ := collectRecentFailedRuns(ctx, svc, "", 20)
	response.Success(c, map[string]any{
		"repo_count":      repoCount,
		"tasks_by_status": taskStatus,
		"recent_failed":   failed,
		"health":          svc.HealthCheck(),
		"generated_at":    time.Now().Format(time.RFC3339),
	})
}
