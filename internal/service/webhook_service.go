package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/yi-nology/git-platform-sdk/pkg/branchfilter"
	"github.com/yi-nology/git-sync-service/internal/dao"
	"github.com/yi-nology/git-sync-service/sync/model"
)

// WebhookService handles webhook-related operations.
type WebhookService struct {
	ruleDAO  *dao.WebhookRuleDAO
	eventDAO *dao.WebhookEventDAO
	repoDAO  *dao.RepoDAO
}

// NewWebhookService creates a new WebhookService instance.
func NewWebhookService(ruleDAO *dao.WebhookRuleDAO, eventDAO *dao.WebhookEventDAO, repoDAO *dao.RepoDAO) *WebhookService {
	return &WebhookService{
		ruleDAO:  ruleDAO,
		eventDAO: eventDAO,
		repoDAO:  repoDAO,
	}
}

// ListRules returns webhook rules for a repository.
func (ws *WebhookService) ListRules(ctx context.Context, repoKey string) ([]*model.WebhookRule, error) {
	return ws.ruleDAO.FindByRepoKey(repoKey)
}

// GetRule returns a webhook rule by ID.
func (ws *WebhookService) GetRule(ctx context.Context, id uint) (*model.WebhookRule, error) {
	return ws.ruleDAO.FindByID(id)
}

// CreateRule creates a new webhook rule.
func (ws *WebhookService) CreateRule(ctx context.Context, req *model.CreateRuleRequest) (*model.WebhookRule, error) {
	rule := &model.WebhookRule{
		Name:          req.Name,
		RepoKey:       req.RepoKey,
		EventType:     req.EventType,
		BranchPattern: req.BranchPattern,
		Action:        req.Action,
		MinInterval:   req.MinInterval,
		Enabled:       req.Enabled,
		Description:   req.Description,
	}
	rule.SetTaskKeys(req.TaskKeys)

	if err := ws.ruleDAO.Create(rule); err != nil {
		return nil, err
	}

	return rule, nil
}

// UpdateRule updates an existing webhook rule.
func (ws *WebhookService) UpdateRule(ctx context.Context, req *model.UpdateRuleRequest) (*model.WebhookRule, error) {
	rule, err := ws.ruleDAO.FindByID(req.ID)
	if err != nil {
		return nil, err
	}
	if rule == nil {
		return nil, ErrRuleNotFound
	}

	if req.Name != "" {
		rule.Name = req.Name
	}
	if req.EventType != "" {
		rule.EventType = req.EventType
	}
	rule.BranchPattern = req.BranchPattern
	if req.Action != "" {
		rule.Action = req.Action
	}
	if req.TaskKeys != nil {
		rule.SetTaskKeys(req.TaskKeys)
	}
	if req.MinInterval > 0 {
		rule.MinInterval = req.MinInterval
	}
	rule.Enabled = req.Enabled
	rule.Description = req.Description

	if err := ws.ruleDAO.Update(rule); err != nil {
		return nil, err
	}

	return rule, nil
}

// DeleteRule deletes a webhook rule by ID.
func (ws *WebhookService) DeleteRule(ctx context.Context, id uint) error {
	return ws.ruleDAO.Delete(id)
}

// ListEvents returns webhook events for a repository.
func (ws *WebhookService) ListEvents(ctx context.Context, repoKey string, offset, limit int) ([]*model.WebhookEvent, int64, error) {
	page := dao.DefaultPagination(offset, limit)
	return ws.eventDAO.FindByRepoKey(repoKey, page)
}

// MarkEventProcessing 查找事件并标记为 processing,返回该事件供后续处理。
func (ws *WebhookService) MarkEventProcessing(ctx context.Context, eventID uint) (*model.WebhookEvent, error) {
	event, err := ws.eventDAO.FindByID(eventID)
	if err != nil {
		return nil, err
	}
	if event == nil {
		return nil, ErrEventNotFound
	}
	event.Status = model.StatusProcessing
	// targeted UPDATE,避免全字段回写大 payload
	if err := ws.eventDAO.UpdateStatus(event.ID, model.StatusProcessing, nil); err != nil {
		slog.Error("failed to update event status to processing", "eventID", eventID, "error", err)
	}
	return event, nil
}

// MarkEventProcessed 标记事件为 processed(处理完成后调用)。
// 用 targeted UPDATE 仅写 status+processed_at,避免全字段回写大 payload。
func (ws *WebhookService) MarkEventProcessed(event *model.WebhookEvent) error {
	now := time.Now()
	event.ProcessedAt = &now
	event.Status = model.StatusProcessed
	if err := ws.eventDAO.UpdateStatus(event.ID, model.StatusProcessed, &now); err != nil {
		slog.Error("failed to update event status to processed", "eventID", event.ID, "error", err)
		return err
	}
	return nil
}

// FindEventByEventID returns a webhook event by event ID (internal use).
func (ws *WebhookService) FindEventByEventID(eventID string) (*model.WebhookEvent, error) {
	return ws.eventDAO.FindByEventID(eventID)
}

// FindEventByID returns a webhook event by ID (internal use).
func (ws *WebhookService) FindEventByID(id uint) (*model.WebhookEvent, error) {
	return ws.eventDAO.FindByID(id)
}

// ApplyRules applies webhook rules to an event.
func (ws *WebhookService) ApplyRules(ctx context.Context, repoKey string, event *model.WebhookEvent, lastTriggerTime *sync.Map, runTaskFn func(ctx context.Context, taskKey, trigger string, webhookEventID *uint) error, webhookEventID *uint) {
	rules, err := ws.ruleDAO.FindByRepoKey(repoKey)
	if err != nil {
		slog.Error("find rules failed", "repoKey", repoKey, "error", err)
		return
	}

	// 缓存已编译的 branch filter,避免同一 pattern 重复编译
	filterCache := make(map[string]*branchfilter.BranchFilter)
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		if !matchEventType(rule.EventType, event.EventType) {
			continue
		}

		compiled, ok := filterCache[rule.BranchPattern]
		if !ok {
			compiled = branchfilter.New(rule.BranchPattern)
			filterCache[rule.BranchPattern] = compiled
		}
		if !compiled.Match(event.Branch) {
			continue
		}

		if rule.Action == model.ActionSync {
			taskKeys := rule.GetTaskKeys()
			for _, taskKey := range taskKeys {
				if taskKey == "" {
					continue
				}
				if rule.MinInterval > 0 {
					key := fmt.Sprintf("%s:%s", rule.RepoKey, taskKey)
					if lastTime, ok := lastTriggerTime.Load(key); ok {
						if t, ok := lastTime.(time.Time); ok && time.Since(t) < time.Duration(rule.MinInterval)*time.Second {
							slog.Warn("skipping due to min interval", "taskKey", taskKey, "minInterval", rule.MinInterval)
							continue
						}
					}
					lastTriggerTime.Store(key, time.Now())
				}
				if err := runTaskFn(ctx, taskKey, model.TriggerWebhook, webhookEventID); err != nil {
					slog.Error("run task failed", "taskKey", taskKey, "error", err)
				}
			}
		}
	}
}

// CreateWebhookEvent creates a new webhook event.
func (ws *WebhookService) CreateWebhookEvent(event *model.WebhookEvent) error {
	return ws.eventDAO.Create(event)
}

// CleanupOldEvents removes webhook events older than the specified duration.
func (ws *WebhookService) CleanupOldEvents(ctx context.Context, maxAge time.Duration) (int64, error) {
	return ws.eventDAO.CleanupOlderThan(ctx, maxAge)
}
