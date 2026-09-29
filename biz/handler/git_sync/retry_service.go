package git_sync

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/biz/model/ops"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// ===== 失败补偿:手动/批量重试 =====

// RetryRun 手动重试一条失败执行:取 run 的 task_key 再触发一次同步。
// 只允许重试 failed 状态,避免把成功任务误触发。
func RetryRun(ctx context.Context, c *app.RequestContext) {
	var req ops.RetryRunReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if uint(req.RunId) == 0 {
		response.BadRequest(c, "run_id is required")
		return
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	run, taskKey, err := findRunByID(ctx, svc, uint(req.RunId))
	if err != nil {
		response.NotFound(c, "run not found")
		return
	}
	if run.Status != "failed" {
		response.BadRequest(c, fmt.Sprintf("only failed runs can be retried, got status %q", run.Status))
		return
	}
	if err := svc.RunTaskAsync(taskKey, "manual_retry", nil); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAudit(ctx, c, "retry", "sync_run", fmt.Sprint(uint(req.RunId)), "手动重试同步 "+taskKey)
	response.Success(c, map[string]any{
		"success":  true,
		"message":  "retry started",
		"task_key": taskKey,
	})
}

// BatchRetryFailed 扫描最近失败执行并批量重跑(返回将要/已重试列表)。
func BatchRetryFailed(ctx context.Context, c *app.RequestContext) {
	var req ops.BatchRetryReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	failed, err := collectRecentFailedRuns(ctx, svc, optStr(req.TaskKey), limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	retried := make([]map[string]any, 0, len(failed))
	skipped := 0
	for _, item := range failed {
		if err := svc.RunTaskAsync(item.taskKey, "batch_retry", nil); err != nil {
			skipped++
			slog.Warn("batch retry skip", "task", item.taskKey, "error", err)
			continue
		}
		retried = append(retried, map[string]any{
			"run_id":   item.runID,
			"task_key": item.taskKey,
		})
	}
	recordAudit(ctx, c, "batch_retry", "sync_run", optStr(req.TaskKey),
		fmt.Sprintf("批量重试 %d 条失败执行", len(retried)))
	response.Success(c, map[string]any{
		"success":   true,
		"retried":   retried,
		"skipped":   skipped,
		"candidate": len(failed),
	})
}

// failedRun 失败执行摘要(重试/概览共用)。
type failedRun struct {
	runID    uint
	taskKey  string
	errorMsg string
	errType  string
	endAt    time.Time
}

func findRunByID(ctx context.Context, svc *corebridge.Service, runID uint) (*corebridge.SyncRun, string, error) {
	const pageSize = 50
	offset := 0
	for page := 0; page < 10; page++ {
		tasks, total, err := svc.ListTasks(ctx, "", offset, pageSize)
		if err != nil {
			return nil, "", err
		}
		for _, t := range tasks {
			runs, _, err := svc.ListHistory(ctx, t.Key, 0, 50)
			if err != nil {
				continue
			}
			for _, r := range runs {
				if r.ID == runID {
					return r, t.Key, nil
				}
			}
		}
		offset += len(tasks)
		if len(tasks) == 0 || int64(offset) >= total {
			break
		}
	}
	return nil, "", fmt.Errorf("run %d not found", runID)
}

func collectRecentFailedRuns(ctx context.Context, svc *corebridge.Service, taskKey string, limit int) ([]failedRun, error) {
	var out []failedRun
	const pageSize = 50
	offset := 0
	for page := 0; page < 10 && len(out) < limit; page++ {
		tasks, total, err := svc.ListTasks(ctx, taskKey, offset, pageSize)
		if err != nil {
			return out, err
		}
		for _, t := range tasks {
			runs, _, err := svc.ListHistory(ctx, t.Key, 0, 10)
			if err != nil {
				continue
			}
			for _, r := range runs {
				if r.Status != "failed" {
					continue
				}
				end := time.Now()
				if r.EndTime != nil {
					end = *r.EndTime
				}
				out = append(out, failedRun{
					runID: r.ID, taskKey: t.Key,
					errorMsg: r.ErrorMessage, errType: r.ErrorType, endAt: end,
				})
				if len(out) >= limit {
					return out, nil
				}
			}
		}
		offset += len(tasks)
		if len(tasks) == 0 || int64(offset) >= total {
			break
		}
	}
	return out, nil
}
