package tools

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/yi-nology/git-ferry-core/health"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/textutil"
)

// ===== 同步健康评分(供 AI 解读) =====

type healthInput struct {
	Limit int `json:"limit,omitempty" jsonschema:"返回条数,默认 10,最大 20"`
}

func (r *Registry) getSyncHealth(ctx context.Context, in healthInput) (string, error) {
	limit := in.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > pageLimit {
		limit = pageLimit
	}
	tasks, _, err := r.svc.ListTasks(ctx, "", 0, limit)
	if err != nil {
		return errJSON("查询任务失败", err), nil
	}
	type dimRow struct {
		Name   string   `json:"name"`
		Score  int      `json:"score"`
		Reason string   `json:"reason,omitempty"`
		Detail []string `json:"detail,omitempty"`
		Action string   `json:"action,omitempty"`
	}
	type row struct {
		Key        string   `json:"key"`
		Name       string   `json:"name"`
		Score      int      `json:"score"`
		Level      string   `json:"level"`
		Issues     []string `json:"issues,omitempty"`
		Actions    []string `json:"actions,omitempty"`
		Dimensions []dimRow `json:"dimensions,omitempty"`
	}
	now := time.Now()
	items := []row{}
	for _, t := range tasks {
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
		runs, _, herr := r.svc.ListHistory(ctx, t.Key, 0, 10)
		if herr == nil && len(runs) > 0 {
			snaps := make([]health.RunSnapshot, 0, len(runs))
			for _, rr := range runs {
				end := time.Time{}
				if rr.EndTime != nil {
					end = *rr.EndTime
				}
				snaps = append(snaps, health.RunSnapshot{
					Status: rr.Status, ErrorMessage: rr.ErrorMessage, EndAt: end,
				})
			}
			for k, v := range health.CollectRunFacts(snaps, now) {
				facts[k] = v
			}
		} else {
			facts["has_history"] = "false"
		}
		res := health.EvaluateDimensions(nil, facts)
		dims := make([]dimRow, 0, len(res.Dimensions))
		for _, d := range res.Dimensions {
			dims = append(dims, dimRow{
				Name: d.Name, Score: d.Score, Reason: d.Reason,
				Detail: d.Detail, Action: d.Action,
			})
		}
		items = append(items, row{
			Key: t.Key, Name: t.Name,
			Score: res.Score, Level: res.Level,
			Issues: res.Issues, Actions: res.Actions, Dimensions: dims,
		})
	}
	return marshalJSON(map[string]any{
		"items": items,
		"legend": map[string]any{
			"dimensions": []string{"reliability", "freshness", "schedule", "safety", "completeness"},
			"levels":     []string{"gold", "silver", "bronze", "basic"},
			"note":       "score<60 的维度会出现在 issues；actions 为建议 CLI 动作",
		},
	}), nil
}

// ===== 仓库资产盘点(孤儿仓库) =====

type inventoryInput struct {
	_ struct{} `json:""`
}

func (r *Registry) getRepoInventory(ctx context.Context, _ inventoryInput) (string, error) {
	repos, _, err := r.svc.ListReposWithFilter(ctx, 0, pageLimit, &corebridge.RepoFilter{})
	if err != nil {
		return errJSON("查询仓库失败", err), nil
	}
	tasks, _, err := r.svc.ListTasks(ctx, "", 0, pageLimit)
	if err != nil {
		return errJSON("查询任务失败", err), nil
	}
	covered := map[string]bool{}
	for _, t := range tasks {
		if t.SourceRepoKey != "" {
			covered[t.SourceRepoKey] = true
		}
		if t.TargetRepoKey != "" {
			covered[t.TargetRepoKey] = true
		}
	}
	type item struct {
		Key      string `json:"key"`
		Name     string `json:"name"`
		HasTask  bool   `json:"has_task"`
		Coverage string `json:"coverage"`
	}
	items := []item{}
	orphans := 0
	for _, rp := range repos {
		has := covered[rp.Key]
		cov := "covered"
		if !has {
			cov = "no_task"
			orphans++
		}
		items = append(items, item{Key: rp.Key, Name: rp.Name, HasTask: has, Coverage: cov})
	}
	return marshalJSON(map[string]any{
		"items": items, "orphan_count": orphans, "total": len(items),
	}), nil
}

// ===== 危险:重试同步 =====

type retryRunInput struct {
	TaskKey string `json:"task_key" jsonschema:"要重试的同步任务 key"`
	Reason  string `json:"reason,omitempty" jsonschema:"重试原因,仅作说明"`
}

func (r *Registry) retrySyncRun(ctx context.Context, in retryRunInput) (string, error) {
	return r.dangerGuard(ctx, "retry_sync_run", marshalJSON(in), func(ctx context.Context, args string) (string, error) {
		var in retryRunInput
		if err := json.Unmarshal([]byte(args), &in); err != nil {
			return marshalJSON(map[string]any{"status": "failed", "error": "参数解析失败: " + err.Error()}), nil
		}
		if err := r.svc.RunTaskWithTrigger(ctx, in.TaskKey, "ai_retry", nil); err != nil {
			return marshalJSON(map[string]any{"status": "failed", "error": "重试失败: " + err.Error()}), nil
		}
		return marshalJSON(map[string]any{
			"status": "ok", "message": "已重新触发同步,详情见任务历史",
			"task_key": in.TaskKey, "at": time.Now().Format(time.RFC3339),
		}), nil
	})
}

// ===== 统一待办队列 =====

type todoInput struct {
	MaxItems int `json:"max_items,omitempty" jsonschema:"最多返回条数,默认 20"`
}

func (r *Registry) getOpsTodo(ctx context.Context, in todoInput) (string, error) {
	limit := in.MaxItems
	if limit <= 0 {
		limit = 20
	}
	// 复用 getSyncHealth 的维度结果 + 孤儿 + RPO
	healthOut, err := r.getSyncHealth(ctx, healthInput{Limit: pageLimit})
	if err != nil {
		return errJSON("健康查询失败", err), nil
	}
	var healthParsed struct {
		Items []struct {
			Key     string   `json:"key"`
			Name    string   `json:"name"`
			Score   int      `json:"score"`
			Issues  []string `json:"issues"`
			Actions []string `json:"actions"`
		} `json:"items"`
	}
	_ = json.Unmarshal([]byte(healthOut), &healthParsed)

	todo := []map[string]any{}
	for _, h := range healthParsed.Items {
		if h.Score >= 60 {
			continue
		}
		pri := 2
		if h.Score < 40 {
			pri = 1
		}
		reason := ""
		if len(h.Issues) > 0 {
			reason = h.Issues[0]
		}
		todo = append(todo, map[string]any{
			"id": "health:" + h.Key, "kind": "health", "priority": pri,
			"task_key": h.Key, "title": "健康分偏低:" + h.Key, "reason": reason,
			"actions": h.Actions,
		})
	}

	// 孤儿
	invOut, _ := r.getRepoInventory(ctx, inventoryInput{})
	var invParsed struct {
		Items []struct {
			Key      string `json:"key"`
			HasTask  bool   `json:"has_task"`
			Coverage string `json:"coverage"`
		} `json:"items"`
	}
	_ = json.Unmarshal([]byte(invOut), &invParsed)
	for _, it := range invParsed.Items {
		if it.HasTask {
			continue
		}
		todo = append(todo, map[string]any{
			"id": "orphan:" + it.Key, "kind": "orphan", "priority": 2,
			"repo_key": it.Key, "title": "仓库无同步任务覆盖",
		})
	}

	// RPO
	rep, rerr := r.svc.RPOReport(86400)
	if rerr == nil && rep != nil {
		for mi := range rep.Metrics {
			m := &rep.Metrics[mi]
			if !m.RPOViolated {
				continue
			}
			todo = append(todo, map[string]any{
				"id": "rpo:" + m.TaskKey, "kind": "rpo", "priority": 1,
				"task_key": m.TaskKey, "title": "备份时效超标", "reason": m.RPOHuman,
			})
		}
	}

	// 排序 priority asc
	sort.SliceStable(todo, func(i, j int) bool {
		pi, _ := todo[i]["priority"].(int)
		pj, _ := todo[j]["priority"].(int)
		return pi < pj
	})
	if len(todo) > limit {
		todo = todo[:limit]
	}
	return marshalJSON(map[string]any{
		"items": todo, "total": len(todo),
		"legend": "priority 1=紧急 2=待处理; kind health|orphan|rpo",
	}), nil
}
