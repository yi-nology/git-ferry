package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/yi-nology/git-platform-sdk/pkg/branchfilter"
	"github.com/yi-nology/git-sync-service/sync/model"
)

// isDuplicateKeyErr 判断错误是否为 DB 唯一约束冲突(MySQL 1062 / SQLite UNIQUE)。
func isDuplicateKeyErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Duplicate entry") || // MySQL
		strings.Contains(msg, "UNIQUE constraint failed") || // SQLite
		strings.Contains(msg, "duplicate key") // PostgreSQL
}

func (s *Service) ReceiveWebhook(ctx context.Context, repoKey string, req *http.Request) error {
	repo, prov, err := s.repos.GetRepoWithProvider(repoKey)
	if err != nil {
		return err
	}

	if err := prov.ValidateWebhookSignature(req, repo.WebhookSecret); err != nil {
		return fmt.Errorf("invalid webhook signature: %w", err)
	}

	event, err := prov.ParseWebhookEvent(req, repo.WebhookSecret)
	if err != nil {
		return fmt.Errorf("parse webhook event failed: %w", err)
	}

	existing, err := s.webhooks.FindEventByEventID(event.ID)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}

	actorName := ""
	if event.Actor != nil {
		actorName = event.Actor.Name
	}

	whEvent := &model.WebhookEvent{
		EventID:   event.ID,
		RepoKey:   repoKey,
		EventType: event.Type,
		Source:    string(event.Source),
		ActorName: actorName,
		Branch:    event.Branch,
		CommitSHA: event.CommitSHA,
		Payload:   event.RawPayload,
		Status:    model.StatusReceived,
	}

	if err := s.webhooks.CreateWebhookEvent(whEvent); err != nil {
		// 并发去重:event_id 有唯一索引,插入冲突说明另一个请求已处理该事件,直接幂等返回。
		// 依赖 DB 唯一约束保证正确性,不再做二次查询(消除竞态窗口+减少一次 DB 往返)。
		if isDuplicateKeyErr(err) {
			return nil
		}
		return err
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.safeApplyRules(s.bgCtx, repoKey, whEvent)
	}()

	return nil
}

func (s *Service) safeApplyRules(ctx context.Context, repoKey string, event *model.WebhookEvent) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in applyRules", "repoKey", repoKey, "error", r)
		}
	}()
	s.applyRules(ctx, repoKey, event)
}

func (s *Service) applyRules(ctx context.Context, repoKey string, event *model.WebhookEvent) {
	eventID := event.ID

	// 正常入站路径必须闭环状态机:received → processing → processed。
	// 此前只在 RetryEvent 里标记 processed,首次接收的事件会永远停在
	// received,历史列表无法区分「待处理」和「已处理完」。
	if _, err := s.webhooks.MarkEventProcessing(ctx, eventID); err != nil {
		slog.Warn("mark event processing failed", "eventID", eventID, "error", err)
	}
	defer func() {
		if err := s.webhooks.MarkEventProcessed(event); err != nil {
			slog.Error("mark event processed failed", "eventID", eventID, "error", err)
		}
	}()

	s.webhooks.ApplyRules(ctx, repoKey, event, &s.lastTriggerTime, func(ctx context.Context, taskKey, trigger string, webhookEventID *uint) error {
		return s.RunTaskWithTrigger(ctx, taskKey, trigger, webhookEventID)
	}, &eventID)
}

func (s *Service) RetryEvent(ctx context.Context, eventID uint) error {
	event, err := s.webhooks.MarkEventProcessing(ctx, eventID)
	if err != nil {
		return err
	}
	// 纳入 WaitGroup + 用 bgCtx,优雅关停时能被等待/取消,不再泄露 goroutine。
	// applyRules 内部会闭环 processing → processed,这里不再重复标记。
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.safeApplyRules(s.bgCtx, event.RepoKey, event)
	}()
	return nil
}

// ListRules returns webhook rules for a repository.
func (s *Service) ListRules(ctx context.Context, repoKey string) ([]*model.WebhookRule, error) {
	return s.webhooks.ListRules(ctx, repoKey)
}

// GetRule returns a webhook rule by ID.
func (s *Service) GetRule(ctx context.Context, id uint) (*model.WebhookRule, error) {
	return s.webhooks.GetRule(ctx, id)
}

// CreateRule creates a new webhook rule.
func (s *Service) CreateRule(ctx context.Context, req *model.CreateRuleRequest) (*model.WebhookRule, error) {
	return s.webhooks.CreateRule(ctx, req)
}

// UpdateRule updates an existing webhook rule.
func (s *Service) UpdateRule(ctx context.Context, req *model.UpdateRuleRequest) (*model.WebhookRule, error) {
	return s.webhooks.UpdateRule(ctx, req)
}

// DeleteRule deletes a webhook rule by ID.
func (s *Service) DeleteRule(ctx context.Context, id uint) error {
	return s.webhooks.DeleteRule(ctx, id)
}

// ListEvents returns webhook events for a repository.
func (s *Service) ListEvents(ctx context.Context, repoKey string, offset, limit int) ([]*model.WebhookEvent, int64, error) {
	return s.webhooks.ListEvents(ctx, repoKey, offset, limit)
}

// matchEventType 事件类型匹配,复用 SDK branchfilter(逗号分隔 glob,
// 空/ "*" 匹配全部;顺带支持 "push*" 等模式)。
func matchEventType(pattern, actual string) bool {
	return branchfilter.New(pattern).Match(actual)
}
