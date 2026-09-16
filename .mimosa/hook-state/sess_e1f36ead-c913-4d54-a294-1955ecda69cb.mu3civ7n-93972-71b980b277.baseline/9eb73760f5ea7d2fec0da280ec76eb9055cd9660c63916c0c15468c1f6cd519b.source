package converter

import (
	"strings"

	webhookmodel "github.com/yi-nology/git-sync-service/biz/model/webhook"
	"github.com/yi-nology/git-sync-core/model"
)

func ToRuleInfo(r *model.WebhookRule) *webhookmodel.WebhookRuleInfo {
	if r == nil {
		return nil
	}
	return &webhookmodel.WebhookRuleInfo{
		ID: SafeUintToInt64(r.ID), Name: r.Name, RepoKey: r.RepoKey,
		EventType: r.EventType, BranchPattern: r.BranchPattern,
		Action: r.Action, SyncTaskKeys: strings.Join(r.GetTaskKeys(), ","),
		MinInterval: SafeIntToInt32(r.MinInterval), Enabled: r.Enabled,
		Description: r.Description, CreatedAt: r.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}

func ToRuleInfoList(rules []*model.WebhookRule) []*webhookmodel.WebhookRuleInfo {
	result := make([]*webhookmodel.WebhookRuleInfo, 0, len(rules))
	for _, r := range rules {
		result = append(result, ToRuleInfo(r))
	}
	return result
}

func ToEventInfo(e *model.WebhookEvent) *webhookmodel.WebhookEventInfo {
	if e == nil {
		return nil
	}
	processedAt := ""
	if e.ProcessedAt != nil {
		processedAt = e.ProcessedAt.Format("2006-01-02 15:04:05")
	}
	return &webhookmodel.WebhookEventInfo{
		ID: SafeUintToInt64(e.ID), EventId: e.EventID, RepoKey: e.RepoKey,
		EventType: e.EventType, Source: e.Source, ActorName: e.ActorName,
		Branch: e.Branch, CommitSha: e.CommitSHA, Status: e.Status,
		ErrorMessage: e.ErrorMessage, ProcessedAt: processedAt,
		CreatedAt: e.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}

func ToEventInfoList(events []*model.WebhookEvent) []*webhookmodel.WebhookEventInfo {
	result := make([]*webhookmodel.WebhookEventInfo, 0, len(events))
	for _, e := range events {
		result = append(result, ToEventInfo(e))
	}
	return result
}
