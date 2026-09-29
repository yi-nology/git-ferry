package git_sync

import (
	"context"
	"fmt"
	"github.com/yi-nology/git-ferry/biz/model/ops"
	"log/slog"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/health"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
	"github.com/yi-nology/git-ferry/internal/tpl"
)

// tplStore 由 main 注入(默认 data/templates.json)。
var tplStore *tpl.Store

// SetTplStore 注入模板库。
func SetTplStore(s *tpl.Store) { tplStore = s }

func requireTplStore(c *app.RequestContext) (*tpl.Store, bool) {
	if tplStore == nil {
		response.Error(c, 503, "template store unavailable")
		return nil, false
	}
	return tplStore, true
}

// ListTemplates GET /api/v1/ops/templates
func ListTemplates(ctx context.Context, c *app.RequestContext) {
	st, ok := requireTplStore(c)
	if !ok {
		return
	}
	if tag := c.Query("tag"); tag != "" {
		response.Success(c, map[string]any{"items": st.ListByTag(tag), "tag": tag})
		return
	}
	response.Success(c, map[string]any{"items": st.List()})
}

// CreateTemplate POST /api/v1/ops/templates
func CreateTemplate(ctx context.Context, c *app.RequestContext) {
	var req tpl.Template
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Name == "" {
		response.BadRequest(c, "name is required")
		return
	}
	st, ok := requireTplStore(c)
	if !ok {
		return
	}
	t, err := st.Upsert(&req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAudit(ctx, c, "create", "template", t.ID, "创建同步策略模板 "+t.Name)
	response.Created(c, t)
}

// DeleteTemplate POST /api/v1/ops/templates/delete?id=
func DeleteTemplate(ctx context.Context, c *app.RequestContext) {
	id := c.Query("id")
	if id == "" {
		response.BadRequest(c, "id is required")
		return
	}
	st, ok := requireTplStore(c)
	if !ok {
		return
	}
	if err := st.Delete(id); err != nil {
		response.NotFound(c, "template not found")
		return
	}
	recordAudit(ctx, c, "delete", "template", id, "删除同步策略模板")
	response.Success(c, map[string]any{"success": true})
}

// PreviewTemplate 套用前预览命中结果(借鉴 Renovate dry-run)。
func PreviewTemplate(ctx context.Context, c *app.RequestContext) {
	var req ops.PreviewTemplateReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	st, ok := requireTplStore(c)
	if !ok {
		return
	}
	t, err := st.Get(req.TemplateId)
	if err != nil {
		response.NotFound(c, "template not found")
		return
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	tasks, _, err := svc.ListTasks(ctx, "", 0, 200)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	filter := matchToFilter(t.Match)
	matched := []map[string]any{}
	for _, task := range tasks {
		if filter.Allow(task.Key, task.Name) {
			matched = append(matched, map[string]any{"key": task.Key, "name": task.Name})
		}
	}
	response.Success(c, map[string]any{
		"template": t,
		"matched":  matched,
		"total":    len(matched),
	})
}

func matchToFilter(m map[string][]string) *health.Filter {
	f := &health.Filter{}
	if m == nil {
		return f
	}
	f.Include = m["include"]
	f.Exclude = m["exclude"]
	f.IncludeGlobs = m["include_globs"]
	f.ExcludeGlobs = m["exclude_globs"]
	return f
}

// ===== 仓库资产盘点 =====

// InventoryItem 仓库资产盘点项:有没有任务覆盖、最近执行。
type InventoryItem struct {
	RepoKey       string   `json:"repo_key"`
	RepoName      string   `json:"repo_name"`
	Platform      string   `json:"platform,omitempty"`
	Status        string   `json:"status,omitempty"`
	HasTask       bool     `json:"has_task"`
	TaskKeys      []string `json:"task_keys,omitempty"`
	LastRunStatus string   `json:"last_run_status,omitempty"`
	LastRunAt     string   `json:"last_run_at,omitempty"`
	Coverage      string   `json:"coverage"` // covered | no_task | stale | failing
}

// RepoInventory GET /api/v1/ops/inventory
// 消灭孤儿仓库:哪些仓库没同步任务、哪些一直失败、哪些很久没跑。
func RepoInventory(ctx context.Context, c *app.RequestContext) {
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	repos, _, err := svc.ListRepos(ctx, 0, 200)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	tasks, _, err := svc.ListTasks(ctx, "", 0, 200)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	// repoKey -> task keys
	taskByRepo := map[string][]string{}
	for _, t := range tasks {
		if t.SourceRepoKey != "" {
			taskByRepo[t.SourceRepoKey] = append(taskByRepo[t.SourceRepoKey], t.Key)
		}
		if t.TargetRepoKey != "" && t.TargetRepoKey != t.SourceRepoKey {
			taskByRepo[t.TargetRepoKey] = append(taskByRepo[t.TargetRepoKey], t.Key)
		}
	}

	items := make([]InventoryItem, 0, len(repos))
	covered, orphan, failing := 0, 0, 0
	for _, r := range repos {
		item := InventoryItem{
			RepoKey:  r.Key,
			RepoName: r.Name,
			Platform: r.Platform,
			Status:   r.Status,
			TaskKeys: taskByRepo[r.Key],
			HasTask:  len(taskByRepo[r.Key]) > 0,
		}
		if !item.HasTask {
			item.Coverage = "no_task"
			orphan++
		} else {
			covered++
			// 取任一任务最近历史
			for _, tk := range item.TaskKeys {
				runs, _, herr := svc.ListHistory(ctx, tk, 0, 1)
				if herr != nil || len(runs) == 0 {
					continue
				}
				item.LastRunStatus = runs[0].Status
				if runs[0].EndTime != nil {
					item.LastRunAt = runs[0].EndTime.Format("2006-01-02 15:04:05")
				}
				if runs[0].Status == "failed" {
					item.Coverage = "failing"
					failing++
					covered--
				}
				break
			}
			if item.Coverage == "" {
				item.Coverage = "covered"
			}
		}
		items = append(items, item)
	}
	response.Success(c, map[string]any{
		"items":        items,
		"total":        len(items),
		"covered":      covered,
		"orphan_repos": orphan,
		"failing":      failing,
	})
}

// ApplyTemplate 批量套用策略模板(借鉴 Renovate packageRules 应用)。
// 只更新已存在任务的 cron/分支/启用位;不隐式创建任务,避免误建。
func ApplyTemplate(ctx context.Context, c *app.RequestContext) {
	var req ops.ApplyTemplateReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	st, ok := requireTplStore(c)
	if !ok {
		return
	}
	t, err := st.Get(req.TemplateId)
	if err != nil {
		response.NotFound(c, "template not found")
		return
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	tasks, _, err := svc.ListTasks(ctx, "", 0, 200)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	filter := matchToFilter(t.Match)
	type change struct {
		Key    string         `json:"key"`
		Name   string         `json:"name"`
		Before map[string]any `json:"before"`
		After  map[string]any `json:"after"`
	}
	changed := []change{}
	for _, task := range tasks {
		if !filter.Allow(task.Key, task.Name) {
			continue
		}
		before := map[string]any{"cron": task.Cron, "enabled": task.Enabled}
		after := map[string]any{"cron": task.Cron, "enabled": task.Enabled}
		need := false
		if t.Spec.Cron != "" && t.Spec.Cron != task.Cron {
			after["cron"] = t.Spec.Cron
			need = true
		}
		if t.Spec.Enabled != nil && *t.Spec.Enabled != task.Enabled {
			after["enabled"] = *t.Spec.Enabled
			need = true
		}
		if !need {
			continue
		}
		if !optBoolDefault(req.DryRun) {
			// UpdateTaskRequest:空字符串=不改;Enabled 指针 nil=不改
			upd := corebridge.UpdateTaskRequest{Key: task.Key, Name: task.Name}
			if t.Spec.Cron != "" {
				upd.Cron = t.Spec.Cron
			}
			if t.Spec.Enabled != nil {
				upd.Enabled = t.Spec.Enabled
			}
			if _, err := svc.UpdateTask(ctx, &upd); err != nil {
				slog.Warn("apply template update task failed", "task", task.Key, "error", err)
				continue
			}
		}
		changed = append(changed, change{Key: task.Key, Name: task.Name, Before: before, After: after})
	}
	recordAudit(ctx, c, "apply_template", "template", t.ID,
		fmt.Sprintf("套用模板 %s,变更 %d 条 (dry_run=%v)", t.Name, len(changed), optBoolDefault(req.DryRun)))
	response.Success(c, map[string]any{
		"template": t,
		"changed":  changed,
		"total":    len(changed),
		"dry_run":  optBoolDefault(req.DryRun),
	})
}
