package tools

import (
	"context"
	"fmt"

	coremodel "github.com/yi-nology/git-sync-core/model"
)

// ===== 执行历史 =====

type listHistoryInput struct {
	TaskKey string `json:"task_key" jsonschema:"同步任务 key"`
	Page    int    `json:"page,omitempty" jsonschema:"页码,从 1 开始,默认 1"`
}

type runSummary struct {
	ID           uint   `json:"id"`
	Status       string `json:"status"`
	Trigger      string `json:"trigger"`
	ErrorMessage string `json:"error_message,omitempty"`
	ErrorType    string `json:"error_type,omitempty"`
	DurationMs   int64  `json:"duration_ms"`
	StartedAt    string `json:"started_at,omitempty"`
}

func toRunSummary(run *coremodel.SyncRun) runSummary {
	s := runSummary{
		ID: run.ID, Status: run.Status, Trigger: run.TriggerSource,
		ErrorMessage: run.ErrorMessage, ErrorType: run.ErrorType,
		DurationMs: run.DurationMs,
	}
	if !run.StartTime.IsZero() {
		s.StartedAt = run.StartTime.Format("2006-01-02 15:04:05")
	}
	return s
}

func (r *Registry) listHistory(ctx context.Context, in listHistoryInput) (string, error) {
	list, total, err := r.svc.ListHistory(ctx, in.TaskKey, pageBounds(in.Page), pageLimit)
	if err != nil {
		return fmt.Sprintf(`{"error":"查询历史失败: %s"}`, err), nil
	}
	runs := []runSummary{}
	for _, run := range list {
		runs = append(runs, toRunSummary(run))
	}
	return marshalJSON(map[string]any{"total": total, "runs": runs}), nil
}

type runDetailInput struct {
	TaskKey string `json:"task_key" jsonschema:"同步任务 key"`
	RunID   uint   `json:"run_id" jsonschema:"执行记录 ID(来自 list_sync_history)"`
}

// getRunDetail 在任务最近历史上(最多翻 5 页)定位 run,返回完整执行日志。
// core Service 未暴露 run+steps 查询(不改 core),而 executor 会把步骤链
// 写进 SyncRun.Details 文本,足以支撑问答与诊断。
func (r *Registry) getRunDetail(ctx context.Context, in runDetailInput) (string, error) {
	const maxScanPages = 5
	for page := 1; page <= maxScanPages; page++ {
		list, _, err := r.svc.ListHistory(ctx, in.TaskKey, pageBounds(page), pageLimit)
		if err != nil {
			return fmt.Sprintf(`{"error":"查询历史失败: %s"}`, err), nil
		}
		for _, run := range list {
			if run.ID != in.RunID {
				continue
			}
			details := run.Details
			if len(details) > 4000 {
				details = details[:4000] + "\n...(已截断)"
			}
			return marshalJSON(map[string]any{
				"run":     toRunSummary(run),
				"details": details,
			}), nil
		}
		if len(list) < pageLimit {
			break
		}
	}
	return fmt.Sprintf(`{"found":false,"message":"最近 %d 条历史中未找到 run_id=%d"}`, maxScanPages*pageLimit, in.RunID), nil
}
