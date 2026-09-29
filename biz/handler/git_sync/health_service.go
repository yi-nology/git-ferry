package git_sync

import (
	"github.com/yi-nology/git-ferry/internal/pkg/textutil"
	"context"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/biz/model/ops"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/health"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// ===== 健康评分 + 同步概览 =====

// HealthScoreItem 单仓库/任务健康分。
type HealthScoreItem struct {
	Key    string   `json:"key"`
	Name   string   `json:"name"`
	Score  int      `json:"score"` // 0-100
	Level  string   `json:"level"` // gold/silver/bronze/basic
	Issues []string `json:"issues,omitempty"`
}

// HealthScore 按规则给任务打分(借鉴 Port Scorecards 的等级模型)。
// 规则(壳层可计算,不依赖 core 扩展):
//   - 有最近成功执行 (+40)
//   - 最近执行无错误 (+30)
//   - 有 cron 或 webhook 触发配置 (+20)
//   - 有任务名/描述元数据 (+10)
func HealthScore(ctx context.Context, c *app.RequestContext) {
	var req ops.HealthScoreReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	items, err := computeHealthScores(ctx, svc, limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, map[string]any{
		"items": items,
		"total": len(items),
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

// healthRules 可替换(测试/后续由配置注入)。
var healthRules = health.DefaultConfig()

func computeHealthScores(ctx context.Context, svc *corebridge.Service, limit int) ([]HealthScoreItem, error) {
	tasks, _, err := svc.ListTasks(ctx, "", 0, limit)
	if err != nil {
		return nil, err
	}
	items := make([]HealthScoreItem, 0, len(tasks))
	for _, t := range tasks {
		facts := health.Facts{
			"has_name": textutil.BoolFact(t.Name != ""),
			"has_cron": textutil.BoolFact(t.Cron != ""),
		}
		runs, _, rerr := svc.ListHistory(ctx, t.Key, 0, 5)
		if rerr == nil && len(runs) > 0 {
			facts["has_history"] = "true"
			last := runs[0]
			facts["recent_success"] = textutil.BoolFact(last.Status == "success")
			facts["error_free"] = textutil.BoolFact(last.ErrorMessage == "")
		} else {
			facts["has_history"] = "false"
		}
		res := healthRules.Evaluate(facts)
		items = append(items, HealthScoreItem{
			Key: t.Key, Name: t.Name,
			Score: res.Score, Level: res.Level, Issues: res.Issues,
		})
	}
	return items, nil
}

