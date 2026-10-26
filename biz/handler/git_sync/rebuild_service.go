package git_sync

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/oekit/pkg/pathx"
	"github.com/yi-nology/git-ferry/biz/model/ops"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// RebuildRepo POST /api/v1/ops/rebuild
// 触发一次「冷启动全量拉取」:由 core 在**拿到该任务的执行权之后**清空工作目录,
// 强制从源重新克隆。
//
// **这里不再自己删目录**:workdir 按 taskKey 共享,受理阶段就删会和正在跑的
// 同步抢同一个目录(把跑着的工作树删掉),而且任务在跑时受理会失败,
// 结果是"目录被删了、重建没排上"。清目录的语义归 core
// (git-ferry-core service/rebuild.go),这里是唯一的 rebuild 语义入口。
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

	if err := svc.RunTaskAsync(req.TaskKey, corebridge.RebuildTrigger, nil); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAudit(ctx, c, "rebuild", "task", req.TaskKey, "从源重建 "+req.TaskKey)
	response.Success(c, map[string]any{
		"success":  true,
		"message":  "full resync started; workdir will be cleared under the task lock",
		"task_key": req.TaskKey,
	})
}

// safeWorkDir 拼 workdir 并拒绝路径穿越(task_key 含 / \ .. 一律拒绝)。
func safeWorkDir(tempDir, taskKey string) (string, error) {
	if taskKey == "" {
		return "", fmt.Errorf("task_key is empty")
	}
	// task_key 是单段名（拼进 workdir 文件名），不是路径——用上游
	// pathx.IsSafeFileName 判定：拒绝路径分隔符（"/" 与 "\"）、"." / ".." 段、
	// 绝对路径与 Windows 盘符。此前是子串判定 Contains("..")，会误杀
	// "foo..bar" 这类合法名字。
	if !pathx.IsSafeFileName(taskKey) {
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
