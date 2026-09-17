package converter

import (
	taskmodel "github.com/yi-nology/git-ferry/biz/model/sync_task"
	"github.com/yi-nology/git-sync-core/model"
)

func ToTaskInfo(t *model.SyncTask) *taskmodel.SyncTaskInfo {
	if t == nil {
		return nil
	}
	lastRunAt := ""
	if t.LastRunAt != nil {
		lastRunAt = t.LastRunAt.Format("2006-01-02 15:04:05")
	}
	return &taskmodel.SyncTaskInfo{
		ID: SafeUintToInt64(t.ID), Key: t.Key, Name: t.Name,
		SourceRepoKey: t.SourceRepoKey, SourceBranch: t.SourceBranch,
		TargetRepoKey: t.TargetRepoKey, TargetBranch: t.TargetBranch,
		SyncMode: t.SyncMode, Cron: t.Cron, WebhookToken: t.WebhookToken,
		Enabled: t.Enabled, GitTags: t.GitTags, GitForce: t.GitForce,
		GitPrune: t.GitPrune,
		LastRunAt: lastRunAt, LastStatus: t.LastStatus,
		CreatedAt: t.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}

func ToTaskInfoList(tasks []*model.SyncTask) []*taskmodel.SyncTaskInfo {
	result := make([]*taskmodel.SyncTaskInfo, 0, len(tasks))
	for _, t := range tasks {
		result = append(result, ToTaskInfo(t))
	}
	return result
}

func ToSyncRunInfo(r *model.SyncRun) *taskmodel.SyncRunInfo {
	if r == nil {
		return nil
	}
	endTime := ""
	if r.EndTime != nil {
		endTime = r.EndTime.Format("2006-01-02 15:04:05")
	}
	info := &taskmodel.SyncRunInfo{
		ID: SafeUintToInt64(r.ID), TaskKey: r.TaskKey, TriggerSource: r.TriggerSource,
		Status: r.Status, StartTime: r.StartTime.Format("2006-01-02 15:04:05"),
		EndTime: endTime, CommitRange: r.CommitRange, Details: r.Details,
		ErrorMessage: r.ErrorMessage, CreatedAt: r.CreatedAt.Format("2006-01-02 15:04:05"),
		DurationMs: r.DurationMs, ErrorType: r.ErrorType, RetryTotal: SafeIntToInt32(r.RetryTotal),
	}
	if r.WebhookEventID != nil {
		id := SafeUintToInt64(*r.WebhookEventID)
		info.WebhookEventId = &id
	}
	if len(r.Steps) > 0 {
		steps := make([]*model.SyncRunStep, len(r.Steps))
		for i := range r.Steps {
			steps[i] = &r.Steps[i]
		}
		info.Steps = ToSyncRunStepInfoList(steps)
	}
	return info
}

func ToSyncRunInfoList(runs []*model.SyncRun) []*taskmodel.SyncRunInfo {
	result := make([]*taskmodel.SyncRunInfo, 0, len(runs))
	for _, r := range runs {
		result = append(result, ToSyncRunInfo(r))
	}
	return result
}

// ToSyncRunInfoListLight 列表视图专用:省略 Details 和 Steps,减少 90%+ 响应体积。
func ToSyncRunInfoListLight(runs []*model.SyncRun) []*taskmodel.SyncRunInfo {
	result := make([]*taskmodel.SyncRunInfo, 0, len(runs))
	for _, r := range runs {
		endTime := ""
		if r.EndTime != nil {
			endTime = r.EndTime.Format("2006-01-02 15:04:05")
		}
		result = append(result, &taskmodel.SyncRunInfo{
			ID: SafeUintToInt64(r.ID), TaskKey: r.TaskKey, TriggerSource: r.TriggerSource,
			Status: r.Status, StartTime: r.StartTime.Format("2006-01-02 15:04:05"),
			EndTime: endTime, CommitRange: r.CommitRange,
			ErrorMessage: r.ErrorMessage, CreatedAt: r.CreatedAt.Format("2006-01-02 15:04:05"),
			DurationMs: r.DurationMs, ErrorType: r.ErrorType, RetryTotal: SafeIntToInt32(r.RetryTotal),
		})
	}
	return result
}

func ToSyncRunStepInfo(s *model.SyncRunStep) *taskmodel.SyncRunStepInfo {
	if s == nil {
		return nil
	}
	info := &taskmodel.SyncRunStepInfo{
		ID:         SafeUintToInt64(s.ID),
		StepName:   s.StepName,
		Status:     s.Status,
		StartTime:  s.StartTime.Format("2006-01-02 15:04:05"),
		DurationMs: s.DurationMs,
		ErrorMsg:   s.ErrorMsg,
		ErrorType:  s.ErrorType,
		RetryCount: SafeIntToInt32(s.RetryCount),
	}
	if s.EndTime != nil {
		info.EndTime = s.EndTime.Format("2006-01-02 15:04:05")
	}
	return info
}

func ToSyncRunStepInfoList(steps []*model.SyncRunStep) []*taskmodel.SyncRunStepInfo {
	result := make([]*taskmodel.SyncRunStepInfo, 0, len(steps))
	for _, s := range steps {
		result = append(result, ToSyncRunStepInfo(s))
	}
	return result
}

func PageToOffset(page, pageSize int32) (int, int) {
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	if page <= 0 {
		page = 1
	}
	return int((page - 1) * pageSize), int(pageSize)
}
