package tools

import (
	"context"

	coremodel "github.com/yi-nology/git-ferry-core/model"
	"github.com/yi-nology/git-ferry/internal/corebridge"
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
		return errJSON("查询历史失败", err), nil
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

// getRunDetail 按 run_id 精确取一条（core 已提供单条查询），返回完整执行日志。
// executor 会把步骤链写进 SyncRun.Details 文本，足以支撑问答与诊断。
func (r *Registry) getRunDetail(ctx context.Context, in runDetailInput) (string, error) {
	run, err := r.svc.GetSyncRunByID(ctx, in.TaskKey, in.RunID)
	if err != nil {
		// 查不到 ≠ 查询失败：前者 found=false，后者必须报错。
		if corebridge.Classify(err) == corebridge.ErrorClassNotFound {
			return msgJSON(map[string]any{"found": false,
				"message": "未找到该执行记录"}), nil
		}
		return errJSON("查询执行记录失败", err), nil
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
