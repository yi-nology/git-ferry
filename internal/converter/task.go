package converter

import (
	"github.com/yi-nology/git-ferry-core/model"
	taskmodel "github.com/yi-nology/git-ferry/biz/model/sync_task"
	"github.com/yi-nology/git-ferry/internal/corebridge"
)

// syncTaskInfoSpec:core SyncTask → DTO SyncTaskInfo 的搬运契约。
var syncTaskInfoSpec = fieldSpec{
	layout: layoutDateTime,
	// WebhookToken 不下发:字段已无消费点(webhook 鉴权走平台签名),
	// 避免每次列表把 secret 形态的 token 拖给所有调用方。
	skip: map[string]bool{"webhooktoken": true},
}

// syncRunInfoSpec:core SyncRun → DTO SyncRunInfo 的搬运契约。
var syncRunInfoSpec = fieldSpec{
	layout: layoutDateTime,
}

// syncRunInfoLightSpec:列表视图专用,与完整视图同一套契约,只是省掉大字段。
//
// 完整视图与列表视图共用一份契约,core 给 SyncRun 加字段时两处一起生效,
// 不再像以前那样要在两个函数里各抄一遍。
var syncRunInfoLightSpec = fieldSpec{
	layout: layoutDateTime,
	// 列表视图省略 Details 与 Steps(响应体积的大头),顺带省掉 WebhookEventId。
	skip: map[string]bool{"details": true, "steps": true, "webhookeventid": true},
}

// syncRunStepInfoSpec:core SyncRunStep → DTO SyncRunStepInfo 的搬运契约。
var syncRunStepInfoSpec = fieldSpec{
	layout: layoutDateTime,
}

// ToTaskInfo 把 core 的 SyncTask 转成对外的 SyncTaskInfo。
//
// 字段搬运由 syncTaskInfoSpec 声明,core 加字段时这里不用改;
// 只有"不搬"(secret)或"要算"(派生)才需要在契约里写一句。
func ToTaskInfo(t *model.SyncTask) *taskmodel.SyncTaskInfo {
	if t == nil {
		return nil
	}
	info := &taskmodel.SyncTaskInfo{}
	mapFields(info, t, syncTaskInfoSpec)
	return info
}

func ToTaskInfoList(tasks []*model.SyncTask) []*taskmodel.SyncTaskInfo {
	result := make([]*taskmodel.SyncTaskInfo, 0, len(tasks))
	for _, t := range tasks {
		result = append(result, ToTaskInfo(t))
	}
	return result
}

// ToSyncRunInfo 把 core 的 SyncRun 转成对外的 SyncRunInfo(含 Steps)。
func ToSyncRunInfo(r *model.SyncRun) *taskmodel.SyncRunInfo {
	if r == nil {
		return nil
	}
	info := &taskmodel.SyncRunInfo{}
	mapFields(info, r, syncRunInfoSpec)
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
		result = append(result, toSyncRunInfoLight(r))
	}
	return result
}

// toSyncRunInfoLight 单条记录的列表视图转换(与 ToSyncRunInfo 共用契约)。
func toSyncRunInfoLight(r *model.SyncRun) *taskmodel.SyncRunInfo {
	if r == nil {
		return nil
	}
	info := &taskmodel.SyncRunInfo{}
	mapFields(info, r, syncRunInfoLightSpec)
	return info
}

// ToSyncRunStepInfo 把 core 的 SyncRunStep 转成对外的 SyncRunStepInfo。
func ToSyncRunStepInfo(s *model.SyncRunStep) *taskmodel.SyncRunStepInfo {
	if s == nil {
		return nil
	}
	info := &taskmodel.SyncRunStepInfo{}
	mapFields(info, s, syncRunStepInfoSpec)
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
