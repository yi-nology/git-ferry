package git_sync

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/yi-nology/git-ferry-core/tpl"
	"github.com/yi-nology/git-ferry/biz/model/ops"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// ===== 同步策略模板 =====
//
// 模板库存储与业务规则（命中预览 / 批量套用 / 资产盘点）在 core：
// Service.Templates / PreviewTemplate / ApplyTemplate / RepoInventory。
// 壳层只做 HTTP 绑定、审计与响应包装。

// ListTemplates GET /api/v1/ops/templates
func ListTemplates(ctx context.Context, c *app.RequestContext) {
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	st := svc.Templates()
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
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	t, err := svc.Templates().Upsert(&req)
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
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	if err := svc.Templates().Delete(id); err != nil {
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
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	t, matched, err := svc.PreviewTemplate(ctx, req.TemplateId)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.Success(c, map[string]any{
		"template": t,
		"matched":  matched,
		"total":    len(matched),
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
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	dryRun := optBoolDefault(req.DryRun)
	res, err := svc.ApplyTemplate(ctx, req.TemplateId, dryRun)
	if err != nil {
		response.FromError(c, err)
		return
	}
	recordAudit(ctx, c, "apply_template", "template", res.Template.ID,
		fmt.Sprintf("套用模板 %s (extends %s),变更 %d 条 (dry_run=%v)",
			res.Template.Name, strings.Join(res.ExtendsChain, "→"), len(res.Changed), dryRun))
	response.Success(c, map[string]any{
		"template":       res.Template,
		"effective_spec": res.Effective,
		"extends_chain":  res.ExtendsChain,
		"changed":        res.Changed,
		"total":          len(res.Changed),
		"dry_run":        dryRun,
	})
}

// ===== 仓库资产盘点 =====

// RepoInventory GET /api/v1/ops/inventory
// 消灭孤儿仓库:哪些仓库没同步任务、哪些一直失败、哪些很久没跑。
// 盘点规则在 core Service.RepoInventory。
func RepoInventory(ctx context.Context, c *app.RequestContext) {
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	res, err := svc.RepoInventory(ctx)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.Success(c, map[string]any{
		"items":        res.Items,
		"total":        len(res.Items),
		"covered":      res.Covered,
		"orphan_repos": res.Orphan,
		"failing":      res.Failing,
	})
}
