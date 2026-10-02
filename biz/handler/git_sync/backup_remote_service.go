package git_sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/yi-nology/git-ferry/internal/pkg/response"
	"github.com/yi-nology/git-ferry/internal/pkg/textutil"
)

// PushBackup POST /api/v1/ops/push-backup
// 把任务 workdir 中的 mirror 推到额外备份远端（github/gitlab 任意 git remote）。
// 不改写 module 身份，纯备份副本；force 按 --force 与任务策略由调用方控制。
func PushBackup(ctx context.Context, c *app.RequestContext) {
	var req struct {
		TaskKey string `json:"task_key"`
		Remote  string `json:"remote"`  // https://github.com/org/repo.git 或 git@...
		RefSpec string `json:"refspec"` // 默认全部: refs/heads/*:refs/heads/*
		Force   bool   `json:"force"`
		DryRun  bool   `json:"dry_run"`
	}
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.TaskKey == "" || strings.TrimSpace(req.Remote) == "" {
		response.BadRequest(c, "task_key and remote are required")
		return
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	task, err := svc.GetTask(ctx, req.TaskKey)
	if err != nil || task == nil {
		response.NotFound(c, "task not found")
		return
	}

	workDir := svc.GetTempDir(req.TaskKey)
	if _, err := os.Stat(workDir); err != nil {
		response.BadRequest(c, "workdir missing (run the task first): "+workDir)
		return
	}
	refspec := req.RefSpec
	if refspec == "" {
		refspec = "+refs/heads/*:refs/heads/*"
	}
	if req.DryRun {
		response.Success(c, map[string]any{
			"dry_run":  true,
			"task_key": req.TaskKey,
			"workdir":  workDir,
			"remote":   req.Remote,
			"refspec":  refspec,
			"force":    req.Force,
		})
		return
	}

	// push 到备份远端(workdir 即 mirror 工作区)：凭证/临时 remote/退避重试由
	// core PushTaskBackup 经 gitbackend 处理(原裸 exec git push 无凭证,https 私有仓必败)。
	out, err := svc.PushTaskBackup(ctx, task, workDir, req.Remote, refspec,
		req.Force || strings.HasPrefix(refspec, "+"))
	if err != nil {
		response.InternalError(c, fmt.Sprintf("push backup failed: %v", err))
		return
	}
	recordAudit(ctx, c, "push_backup", "task", req.TaskKey, "备份推送 → "+req.Remote)
	response.Success(c, map[string]any{
		"success":  true,
		"task_key": req.TaskKey,
		"remote":   req.Remote,
		"refspec":  refspec,
		"output":   textutil.TruncateRunes(out, 2000),
	})
}

// RepoFiles GET /api/v1/ops/repo-files?task_key=&path=
// 只读浏览任务 workdir 中的文件/目录（救援用）。path 相对 workdir，禁止 ..。
func RepoFiles(ctx context.Context, c *app.RequestContext) {
	taskKey := c.Query("task_key")
	if taskKey == "" {
		response.BadRequest(c, "task_key is required")
		return
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	root := svc.GetTempDir(taskKey)
	if _, err := os.Stat(root); err != nil {
		response.NotFound(c, "workdir missing (not synced yet)")
		return
	}
	rel := c.Query("path")
	if rel == "" {
		rel = "."
	}
	if strings.Contains(rel, "..") || filepath.IsAbs(rel) {
		response.BadRequest(c, "path must be relative and without ..")
		return
	}
	full := filepath.Join(root, filepath.Clean(rel))
	// 双保险：解析后必须仍在 root 下
	if absRoot, err := filepath.Abs(root); err == nil {
		if absFull, err2 := filepath.Abs(full); err2 == nil && !strings.HasPrefix(absFull, absRoot) {
			response.BadRequest(c, "path escapes workdir")
			return
		}
	}
	info, err := os.Stat(full)
	if err != nil {
		response.NotFound(c, "not found")
		return
	}
	if !info.IsDir() {
		// 单文件：返回元数据 + 前 4KB 预览
		b, rerr := os.ReadFile(full) //nolint:gosec // 路径已限定在 workdir
		if rerr != nil {
			response.InternalError(c, rerr.Error())
			return
		}
		preview := b
		if len(preview) > 4096 {
			preview = preview[:4096]
		}
		response.Success(c, map[string]any{
			"type":    "file",
			"path":    rel,
			"size":    info.Size(),
			"preview": string(preview),
		})
		return
	}
	entries, err := os.ReadDir(full)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	type item struct {
		Name  string `json:"name"`
		IsDir bool   `json:"is_dir"`
		Size  int64  `json:"size"`
	}
	items := make([]item, 0, len(entries))
	for i := range entries {
		e := entries[i]
		size := int64(0)
		if !e.IsDir() {
			if fi, err := e.Info(); err == nil {
				size = fi.Size()
			}
		}
		items = append(items, item{Name: e.Name(), IsDir: e.IsDir(), Size: size})
	}
	response.Success(c, map[string]any{
		"type":  "dir",
		"path":  rel,
		"task":  taskKey,
		"items": items,
	})
}
