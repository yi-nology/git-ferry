package dao

import (
	"context"
	"errors"
	"time"

	"github.com/yi-nology/git-sync-service/sync/model"
	"gorm.io/gorm"
)

type WebhookEventDAO struct {
	db *gorm.DB
}

func NewWebhookEventDAO(db *gorm.DB) *WebhookEventDAO {
	return &WebhookEventDAO{db: db}
}

func (d *WebhookEventDAO) FindByEventID(eventID string) (*model.WebhookEvent, error) {
	var event model.WebhookEvent
	err := d.db.Where("event_id = ?", eventID).First(&event).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &event, err
}

func (d *WebhookEventDAO) FindByRepoKey(repoKey string, page Pagination) ([]*model.WebhookEvent, int64, error) {
	var events []*model.WebhookEvent
	query := d.db.Where("repo_key = ?", repoKey)
	total, err := Paginate(query, page, &events)
	return events, total, err
}

func (d *WebhookEventDAO) Create(event *model.WebhookEvent) error {
	return d.db.Create(event).Error
}

func (d *WebhookEventDAO) Update(event *model.WebhookEvent) error {
	return d.db.Save(event).Error
}

// UpdateStatus 仅更新 status 和 processed_at 字段,避免全字段回写(payload 可能很大)。
func (d *WebhookEventDAO) UpdateStatus(id uint, status string, processedAt *time.Time) error {
	return d.db.Model(&model.WebhookEvent{}).Where("id = ?", id).
		Updates(map[string]interface{}{"status": status, "processed_at": processedAt}).Error
}

func (d *WebhookEventDAO) FindByID(id uint) (*model.WebhookEvent, error) {
	return FindByID[model.WebhookEvent](d.db, id)
}

func (d *WebhookEventDAO) FindRecent(repoKey string, page Pagination) ([]*model.WebhookEvent, error) {
	var events []*model.WebhookEvent
	query := d.db.Offset(page.Offset).Limit(page.Limit).Order("id DESC")
	if repoKey != "" {
		query = query.Where("repo_key = ?", repoKey)
	}
	err := query.Find(&events).Error
	return events, err
}

func (d *WebhookEventDAO) CleanupOlderThan(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	var total int64
	for {
		if ctx.Err() != nil {
			return total, ctx.Err()
		}
		result := d.db.WithContext(ctx).Where("created_at < ?", cutoff).Limit(1000).Delete(&model.WebhookEvent{})
		if result.Error != nil {
			return total, result.Error
		}
		total += result.RowsAffected
		if result.RowsAffected == 0 {
			break
		}
	}
	return total, nil
}

func (d *WebhookEventDAO) CountByRepoKey(repoKey string) (int64, error) {
	var count int64
	err := d.db.Model(&model.WebhookEvent{}).Where("repo_key = ?", repoKey).Count(&count).Error
	return count, err
}
