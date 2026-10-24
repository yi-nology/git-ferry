package converter

import (
	"strings"

	"github.com/yi-nology/git-ferry-core/model"
	webhookmodel "github.com/yi-nology/git-ferry/biz/model/webhook"
)

// webhookRuleInfoSpec:core WebhookRule → DTO WebhookRuleInfo 的搬运契约。
var webhookRuleInfoSpec = fieldSpec{
	layout: layoutDateTime,
	// SyncTaskKeys 是把关联的任务表拍平成逗号分隔串,不是 core 上的字段。
	derived: map[string]bool{"synctaskkeys": true},
}

// webhookEventInfoSpec:core WebhookEvent → DTO WebhookEventInfo 的搬运契约。
var webhookEventInfoSpec = fieldSpec{
	layout: layoutDateTime,
}

// ToRuleInfo 把 core 的 WebhookRule 转成对外的 WebhookRuleInfo。
func ToRuleInfo(r *model.WebhookRule) *webhookmodel.WebhookRuleInfo {
	if r == nil {
		return nil
	}
	info := &webhookmodel.WebhookRuleInfo{}
	mapFields(info, r, webhookRuleInfoSpec)
	// 派生字段:关联任务拍平成 "key1,key2"。
	info.SyncTaskKeys = strings.Join(r.GetTaskKeys(), ",")
	return info
}

func ToRuleInfoList(rules []*model.WebhookRule) []*webhookmodel.WebhookRuleInfo {
	result := make([]*webhookmodel.WebhookRuleInfo, 0, len(rules))
	for _, r := range rules {
		result = append(result, ToRuleInfo(r))
	}
	return result
}

// ToEventInfo 把 core 的 WebhookEvent 转成对外的 WebhookEventInfo。
func ToEventInfo(e *model.WebhookEvent) *webhookmodel.WebhookEventInfo {
	if e == nil {
		return nil
	}
	info := &webhookmodel.WebhookEventInfo{}
	mapFields(info, e, webhookEventInfoSpec)
	return info
}

func ToEventInfoList(events []*model.WebhookEvent) []*webhookmodel.WebhookEventInfo {
	result := make([]*webhookmodel.WebhookEventInfo, 0, len(events))
	for _, e := range events {
		result = append(result, ToEventInfo(e))
	}
	return result
}
