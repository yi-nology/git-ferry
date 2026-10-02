package git_sync

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/yi-nology/git-ferry/internal/pkg/response"
	"github.com/yi-nology/git-ferry/internal/pkg/textutil"
)

// force-push 审批（gitea-mirror block-and-approve 模式）：
// force_push_policy=block 时，目标分歧触发的覆盖请求进入 pending 列表，
// Admin 一次性放行后允许本次 force。
// 存储与放行判定在 core Service.ForcePushApprovals()（<backup_dir>/force-push-approvals.json）；
// 执行器的回调默认就是该存储，壳只提供 HTTP 端点与操作者身份。

// ListForcePushApprovals GET /api/v1/ops/force-push-approvals
func ListForcePushApprovals(_ context.Context, c *app.RequestContext) {
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	st := svc.ForcePushApprovals()
	list := st.List()
	response.Success(c, map[string]any{"items": list, "total": len(list), "pending": st.PendingCount()})
}

// RequestForcePushApproval POST /api/v1/ops/force-push-approvals
// 产生一条 pending 审批（同步引擎/前端在 block 策略触发时调用）。
func RequestForcePushApproval(ctx context.Context, c *app.RequestContext) {
	var req struct {
		TaskKey string `json:"task_key"`
		Branch  string `json:"branch"`
		Reason  string `json:"reason"`
	}
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.TaskKey == "" {
		response.BadRequest(c, "task_key is required")
		return
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	item, err := svc.ForcePushApprovals().Request(req.TaskKey, req.Branch, req.Reason)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAudit(ctx, c, "force_push_request", "task", req.TaskKey, "强制推送审批申请 "+req.Branch)
	response.Created(c, item)
}

// ApproveForcePush POST /api/v1/ops/force-push-approvals/approve
// Admin 一次性放行指定审批。
func ApproveForcePush(ctx context.Context, c *app.RequestContext) {
	var req struct {
		ID string `json:"id"`
	}
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.ID == "" {
		response.BadRequest(c, "id is required")
		return
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	found, err := svc.ForcePushApprovals().Approve(req.ID, actorName(c))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if !found {
		response.NotFound(c, "approval not found")
		return
	}
	recordAudit(ctx, c, "force_push_approve", "task", req.ID, "放行强制推送 "+req.ID)
	response.Success(c, map[string]any{"approved": true, "id": req.ID})
}

// actorName 从请求头推断操作者（API Key 只保留前 8 位做展示）。
func actorName(c *app.RequestContext) string {
	if v := string(c.GetHeader("X-API-Key")); v != "" {
		return "apikey:" + textutil.SanitizePathToken(v[:minInt(8, len(v))])
	}
	return "admin"
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
