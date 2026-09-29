package tools

import (
	"github.com/yi-nology/git-ferry/internal/pkg/textutil"
	"context"
	"encoding/json"
	"time"

	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/health"
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
	rules := health.DefaultConfig()
	type row struct {
		Key    string   `json:"key"`
		Name   string   `json:"name"`
		Score  int      `json:"score"`
		Level  string   `json:"level"`
		Issues []string `json:"issues,omitempty"`
	}
	items := []row{}
	for _, t := range tasks {
		facts := health.Facts{"has_name": textutil.BoolFact(t.Name != ""), "has_cron": textutil.BoolFact(t.Cron != "")}
		runs, _, herr := r.svc.ListHistory(ctx, t.Key, 0, 3)
		if herr == nil && len(runs) > 0 {
			facts["has_history"] = "true"
			facts["recent_success"] = textutil.BoolFact(runs[0].Status == "success")
			facts["error_free"] = textutil.BoolFact(runs[0].ErrorMessage == "")
		}
		res := rules.Evaluate(facts)
		items = append(items, row{Key: t.Key, Name: t.Name, Score: res.Score, Level: res.Level, Issues: res.Issues})
	}
	return marshalJSON(map[string]any{"items": items}), nil
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

