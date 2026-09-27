package git_sync

import (
	"context"
	"os"
	"path/filepath"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// RebuildReq 一键从源重建(借鉴 ghorg reclone):清掉本地 workdir 再全量同步。
type RebuildReq struct {
	TaskKey string `json:"task_key" form:"task_key" query:"task_key"`
}

// RebuildRepo POST /api/v1/ops/rebuild
// 删除任务临时工作目录后触发一次同步,等价于「冷启动全量拉取」。
// 用于 workdir 损坏、浅克隆修复、或需要强制全量刷新时。
func RebuildRepo(ctx context.Context, c *app.RequestContext) {
	var req RebuildReq
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
	task, err := svc.GetTask(ctx, req.TaskKey)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	// core FindByKey 未命中返回 (nil, nil),不是 error
	if task == nil {
		response.NotFound(c, "task not found")
		return
	}

	// workdir 路径与 core.GetTempDir 一致:<temp>/<taskKey>
	tempDir := ""
	if cfg := svc.GetConfig(); cfg != nil {
		tempDir = cfg.Git.TempDir
	}
	if tempDir != "" {
		workDir := filepath.Join(tempDir, req.TaskKey)
		if err := os.RemoveAll(workDir); err != nil {
			response.InternalError(c, err.Error())
			return
		}
	}

	if err := svc.RunTaskAsync(req.TaskKey, "rebuild", nil); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAudit(ctx, c, "rebuild", "task", req.TaskKey, "从源重建 "+req.TaskKey)
	response.Success(c, map[string]any{
		"success":  true,
		"message":  "workdir cleared, full resync started",
		"task_key": req.TaskKey,
	})
}
