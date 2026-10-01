package git_sync

import (
	"context"
	"sort"
	"time"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/yi-nology/git-ferry/internal/health"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// OpsTodoItem 统一待办项（Renovate Dependency Dashboard 模式）。
type OpsTodoItem struct {
	ID       string          `json:"id"`
	Kind     string          `json:"kind"`     // health | orphan | rpo | drift
	Priority int             `json:"priority"` // 1=紧急
	TaskKey  string          `json:"task_key,omitempty"`
	RepoKey  string          `json:"repo_key,omitempty"`
	Title    string          `json:"title"`
	Reason   string          `json:"reason,omitempty"`
	Actions  []health.Action `json:"actions,omitempty"`
}

// OpsTodo GET /api/v1/ops/todo
// 聚合健康 attention + 孤儿仓库 + RPO 超标，按优先级输出可执行队列。
func OpsTodo(ctx context.Context, c *app.RequestContext) {
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	items := []OpsTodoItem{}

	// 1) 健康 attention
	healthItems, err := computeHealthScores(ctx, svc, 100, nil)
	if err == nil {
		for hi := range healthItems {
			h := &healthItems[hi]
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
			items = append(items, OpsTodoItem{
				ID:       "health:" + h.Key,
				Kind:     "health",
				Priority: pri,
				TaskKey:  h.Key,
				Title:    "健康分偏低：" + nameOr(h.Name, h.Key),
				Reason:   reason,
				Actions:  h.ActionItems,
			})
		}
	}

	// 2) 孤儿仓库
	repos, _, rerr := svc.ListRepos(ctx, 0, 200)
	tasks, _, terr := svc.ListTasks(ctx, "", 0, 200)
	if rerr == nil && terr == nil {
		taskByRepo := map[string]bool{}
		for _, t := range tasks {
			if t.SourceRepoKey != "" {
				taskByRepo[t.SourceRepoKey] = true
			}
			if t.TargetRepoKey != "" {
				taskByRepo[t.TargetRepoKey] = true
			}
		}
		for _, r := range repos {
			if taskByRepo[r.Key] {
				continue
			}
			items = append(items, OpsTodoItem{
				ID:       "orphan:" + r.Key,
				Kind:     "orphan",
				Priority: 2,
				RepoKey:  r.Key,
				Title:    "仓库无同步任务覆盖",
				Reason:   r.Name,
				Actions: []health.Action{{
					Kind: "cli", Dimension: "coverage", Priority: 2,
					Title:   "创建同步任务",
					Command: "gitferry task +create --name ... --source-repo " + r.Key + " --target-repo ...",
				}},
			})
		}
	}

	// 3) RPO 超标
	if rep, perr := svc.RPOReport(86400); perr == nil && rep != nil {
		for mi := range rep.Metrics {
			m := &rep.Metrics[mi]
			if !m.RPOViolated {
				continue
			}
			items = append(items, OpsTodoItem{
				ID:       "rpo:" + m.TaskKey,
				Kind:     "rpo",
				Priority: 1,
				TaskKey:  m.TaskKey,
				Title:    "备份时效超标（RPO）",
				Reason:   m.RPOHuman,
				Actions: []health.Action{{
					Kind: "cli", Dimension: "freshness", Priority: 1,
					Title:   "查看 RPO 明细",
					Command: "gitferry ops +rpo --max-seconds 86400 --format json",
				}},
			})
		}
	}

	// 排序：优先级 → kind → key
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Priority != items[j].Priority {
			return items[i].Priority < items[j].Priority
		}
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		return items[i].ID < items[j].ID
	})

	byKind := map[string]int{}
	for i := range items {
		byKind[items[i].Kind]++
	}
	response.Success(c, map[string]any{
		"items":        items,
		"total":        len(items),
		"by_kind":      byKind,
		"generated_at": time.Now().Format(time.RFC3339),
	})
}

func nameOr(name, fallback string) string {
	if name != "" {
		return name
	}
	return fallback
}
