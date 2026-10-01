package git_sync

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/biz/model/ops"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/health"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
	"github.com/yi-nology/git-ferry/internal/pkg/textutil"
)

// ===== 健康评分 + 同步概览 =====

// HealthScoreItem 单仓库/任务健康分（含 Scorecards 风格维度）。
type HealthScoreItem struct {
	Key    string   `json:"key"`
	Name   string   `json:"name"`
	Score  int      `json:"score"` // 0-100
	Level  string   `json:"level"` // gold/silver/bronze/basic
	Issues []string `json:"issues,omitempty"`
	// Dimensions 分项：reliability / freshness / schedule / safety / completeness
	Dimensions []health.Dimension `json:"dimensions,omitempty"`
	// Actions 建议动作（CLI 命令或人工步骤）
	Actions []string `json:"actions,omitempty"`
	// ActionItems 结构化动作（带可复制命令）
	ActionItems []health.Action `json:"action_items,omitempty"`
}

// HealthScore 按维度规则给任务打分(借鉴 OpenSSF Scorecards)。
// 维度：reliability(35) / freshness(20) / schedule(15) / safety(15) / completeness(15)
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
	items, err := computeHealthScores(ctx, svc, limit, driftByTask)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	// 聚合：按分数升序（最差在前，便于「需要关注」）
	levels := map[string]int{}
	for i := range items {
		levels[items[i].Level]++
	}
	// 待办动作去重汇总（Renovate Dependency Dashboard 模式）
	actionSet := map[string]int{}
	var topActions []string
	for i := range items {
		for _, a := range items[i].Actions {
			actionSet[a]++
		}
	}
	for a, n := range actionSet {
		topActions = append(topActions, fmt.Sprintf("%s（×%d）", a, n))
	}
	sort.Strings(topActions)

	attention := []HealthScoreItem{}
	for i := range items {
		if items[i].Score < 60 {
			attention = append(attention, items[i])
		}
	}
	response.Success(c, map[string]any{
		"items": items,
		"total": len(items),
		"summary": map[string]any{
			"levels":       levels,
			"below_silver": len(attention),
			"top_actions":  topActions,
		},
		"attention":    attention,
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

// healthRules 可替换(测试/后续由配置注入)；维度规格同理。
var (
	healthRules = health.DefaultConfig()
	healthDims  = health.DefaultDimensions()
)

func computeHealthScores(ctx context.Context, svc *corebridge.Service, limit int, driftByTask map[string]int) ([]HealthScoreItem, error) {
	tasks, _, err := svc.ListTasks(ctx, "", 0, limit)
	if err != nil {
		return nil, err
	}
	now := time.Now()

	// 并行拉历史，消除 N+1（goroutine 上限 8）
	type histResult struct {
		runs []*corebridge.SyncRun
		err  error
	}
	histCh := make([]chan histResult, len(tasks))
	sem := make(chan struct{}, 8)
	for i := range tasks {
		ch := make(chan histResult, 1)
		histCh[i] = ch
		go func(i int, taskKey string) {
			sem <- struct{}{}
			defer func() { <-sem }()
			runs, _, herr := svc.ListHistory(ctx, taskKey, 0, 10)
			ch <- histResult{runs: runs, err: herr}
		}(i, tasks[i].Key)
	}

	items := make([]HealthScoreItem, 0, len(tasks))
	for i, t := range tasks {
		facts := health.Facts{
			"has_name":          textutil.BoolFact(t.Name != ""),
			"has_cron":          textutil.BoolFact(t.Cron != ""),
			"cron":              t.Cron,
			"enabled":           textutil.BoolFact(t.Enabled),
			"keep_divergent":    textutil.BoolFact(t.KeepDivergent),
			"git_force":         textutil.BoolFact(t.GitForce),
			"force_push_policy": t.ForcePushPolicy,
			"backup_enabled":    textutil.BoolFact(t.GitBundle),
			"sync_wiki":         textutil.BoolFact(t.SyncWiki),
		}
		if n, ok := driftByTask[t.Key]; ok && n > 0 {
			facts["drift_count"] = textutil.Itoa(n)
		}

		hr := <-histCh[i]
		runs := hr.runs
		if hr.err == nil && len(runs) > 0 {
			snaps := make([]health.RunSnapshot, 0, len(runs))
			var lastFailRunID int64
			for _, r := range runs {
				end := time.Time{}
				if r.EndTime != nil {
					end = *r.EndTime
				}
				stepFails := 0
				for si := range r.Steps {
					if r.Steps[si].Status == "failed" {
						stepFails++
					}
				}
				snaps = append(snaps, health.RunSnapshot{
					Status:       r.Status,
					ErrorMessage: r.ErrorMessage,
					EndAt:        end,
					StepFails:    stepFails,
					RetryTotal:   r.RetryTotal,
					ErrorType:    r.ErrorType,
				})
				if lastFailRunID == 0 && r.Status == "failed" {
					lastFailRunID = int64(r.ID)
				}
			}
			for k, v := range health.CollectRunFacts(snaps, now) {
				facts[k] = v
			}
			// 动作路由需要最近失败 run_id
			_ = lastFailRunID
		} else {
			facts["has_history"] = "false"
		}

		dimRes := health.EvaluateDimensions(healthDims, facts)
		legacy := healthRules.Evaluate(facts)
		issues := uniqueStrings(append(legacy.Issues, dimRes.Issues...))

		// 最近失败 run_id（动作路由）
		var failRunID int64
		if hr.err == nil {
			for _, r := range hr.runs {
				if r.Status == "failed" {
					failRunID = int64(r.ID)
					break
				}
			}
		}
		actionItems := health.RouteActions(health.TaskBrief{
			TaskKey:  t.Key,
			TaskName: t.Name,
			RunID:    failRunID,
			DriftN:   driftByTask[t.Key],
		}, dimRes.Dimensions)

		items = append(items, HealthScoreItem{
			Key:         t.Key,
			Name:        t.Name,
			Score:       dimRes.Score,
			Level:       dimRes.Level,
			Issues:      issues,
			Dimensions:  dimRes.Dimensions,
			Actions:     dimRes.Actions,
			ActionItems: actionItems,
		})
	}
	return items, nil
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
