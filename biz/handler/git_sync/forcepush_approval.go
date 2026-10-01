package git_sync

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
	"github.com/yi-nology/git-ferry/internal/pkg/textutil"
)

// force-push 审批（gitea-mirror block-and-approve 模式）：
// force_push_policy=block 时，目标分歧触发的覆盖请求进入 pending 列表，
// Admin 一次性放行后允许本次 force。持久化在 backup_dir/force-push-approvals.json。

type forcePushApproval struct {
	ID         string     `json:"id"`
	TaskKey    string     `json:"task_key"`
	Branch     string     `json:"branch"`
	Reason     string     `json:"reason"`
	CreatedAt  time.Time  `json:"created_at"`
	Approved   bool       `json:"approved"`
	ApprovedBy string     `json:"approved_by,omitempty"`
	ApprovedAt *time.Time `json:"approved_at,omitempty"`
}

func forcePushApprovalPath() string {
	svc := GetSyncService()
	if svc == nil || svc.BackupDir() == "" {
		return filepath.Join("data", "force-push-approvals.json")
	}
	return filepath.Join(svc.BackupDir(), "force-push-approvals.json")
}

func loadForcePushApprovals() []forcePushApproval {
	data, err := os.ReadFile(forcePushApprovalPath()) //nolint:gosec // 内部状态文件
	if err != nil {
		return []forcePushApproval{}
	}
	var list []forcePushApproval
	if json.Unmarshal(data, &list) != nil {
		return []forcePushApproval{}
	}
	return list
}

func saveForcePushApprovals(list []forcePushApproval) error {
	path := forcePushApprovalPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// ListForcePushApprovals GET /api/v1/ops/force-push-approvals
func ListForcePushApprovals(_ context.Context, c *app.RequestContext) {
	list := loadForcePushApprovals()
	pending := 0
	for _, a := range list {
		if !a.Approved {
			pending++
		}
	}
	response.Success(c, map[string]any{"items": list, "total": len(list), "pending": pending})
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
	list := loadForcePushApprovals()
	item := forcePushApproval{
		ID:        fmt.Sprintf("fp-%d", time.Now().UnixNano()),
		TaskKey:   req.TaskKey,
		Branch:    req.Branch,
		Reason:    req.Reason,
		CreatedAt: time.Now().UTC(),
	}
	list = append(list, item)
	if err := saveForcePushApprovals(list); err != nil {
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
	list := loadForcePushApprovals()
	found := false
	now := time.Now().UTC()
	for i := range list {
		if list[i].ID != req.ID {
			continue
		}
		list[i].Approved = true
		list[i].ApprovedBy = actorName(c)
		list[i].ApprovedAt = &now
		found = true
		break
	}
	if !found {
		response.NotFound(c, "approval not found")
		return
	}
	if err := saveForcePushApprovals(list); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAudit(ctx, c, "force_push_approve", "task", req.ID, "放行强制推送 "+req.ID)
	response.Success(c, map[string]any{"approved": true, "id": req.ID})
}

// IsForcePushApproved 查询任务+分支是否已放行（供执行器/前端）。
func IsForcePushApproved(taskKey, branch string) bool {
	for _, a := range loadForcePushApprovals() {
		if a.TaskKey == taskKey && a.Branch == branch && a.Approved {
			return true
		}
	}
	return false
}

// ShellForcePushApprover 实现 core executor.ForcePushApprover。
type ShellForcePushApprover struct{}

func (ShellForcePushApprover) IsForcePushApproved(taskKey, branch string) bool {
	return IsForcePushApproved(taskKey, branch)
}

func (ShellForcePushApprover) RequestApproval(taskKey, branch, reason string) {
	list := loadForcePushApprovals()
	// 幂等：同 task+branch 已有 pending 则跳过
	for _, a := range list {
		if a.TaskKey == taskKey && a.Branch == branch && !a.Approved {
			return
		}
	}
	list = append(list, forcePushApproval{
		ID:        fmt.Sprintf("fp-%d", time.Now().UnixNano()),
		TaskKey:   taskKey,
		Branch:    branch,
		Reason:    reason,
		CreatedAt: time.Now().UTC(),
	})
	_ = saveForcePushApprovals(list)
}

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
