package git_sync

import (
	"context"
	"fmt"
	"github.com/yi-nology/git-ferry/biz/model/ops"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// RebuildRepo POST /api/v1/ops/rebuild
// 删除任务临时工作目录后触发一次同步,等价于「冷启动全量拉取」。
// 用于 workdir 损坏、浅克隆修复、或需要强制全量刷新时。
func RebuildRepo(ctx context.Context, c *app.RequestContext) {
	var req ops.RebuildReq
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
		workDir, err := safeWorkDir(tempDir, req.TaskKey)
		if err != nil {
			response.BadRequest(c, err.Error())
			return
		}
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

// safeWorkDir 拼 workdir 并拒绝路径穿越(task_key 含 / \ .. 一律拒绝)。
func safeWorkDir(tempDir, taskKey string) (string, error) {
	if taskKey == "" {
		return "", fmt.Errorf("task_key is empty")
	}
	if strings.ContainsAny(taskKey, "/\\") || strings.Contains(taskKey, "..") {
		return "", fmt.Errorf("task_key contains path separators")
	}
	dir := filepath.Join(tempDir, taskKey)
	// 双保险:结果必须仍在 tempDir 之下
	absTemp, err := filepath.Abs(tempDir)
	if err != nil {
		return "", err
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(absDir, absTemp+string(filepath.Separator)) {
		return "", fmt.Errorf("task_key escapes temp dir")
	}
	return absDir, nil
}
