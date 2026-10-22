package converter

import (
	"github.com/yi-nology/git-ferry-core/model"
	taskmodel "github.com/yi-nology/git-ferry/biz/model/sync_task"
	"github.com/yi-nology/git-ferry/internal/corebridge"
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
		SyncMode: t.SyncMode, Cron: t.Cron,
		// WebhookToken 不下发:字段已无消费点(webhook 鉴权走平台签名),
		// 避免每次列表把 secret 形态的 token 拖给所有调用方
		Enabled: t.Enabled, GitTags: t.GitTags, GitForce: t.GitForce,
		GitPrune: t.GitPrune,
		GitLfs:   t.GitLFS, SyncWiki: t.SyncWiki, GitBundle: t.GitBundle, Submodules: t.Submodules, GitPushPrune: t.GitPushPrune, KeepDivergent: t.KeepDivergent,
		ForcePushPolicy: t.ForcePushPolicy, IncludeBranches: t.IncludeBranches, ExcludeRefPatterns: t.ExcludeRefPatterns,
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

// PageToOffset 把 page/pageSize 换算成 offset/limit。
//
// 页大小的缺省与上限由 core dao 的口径决定（dao.DefaultPagination）——
// 本包不自己写死 50/200，避免与 core 各留一份。这里先用 DefaultPagination
// 把 pageSize 钳好，**再**用钳后的值算 offset：顺序反了的话
// 「页大小超上限」时 offset 会算在未钳的页大小上。
//
// 只依赖 corebridge 的转发入口（core 的 dao 口径），不直接引用 core 的
// 新增常量——core 尚未发版时壳层仍能编译。
func PageToOffset(page, pageSize int32) (int, int) {
	p := corebridge.DefaultPagination(0, int(pageSize))
	if page <= 0 {
		page = 1
	}
	return (int(page) - 1) * p.Limit, p.Limit
}
