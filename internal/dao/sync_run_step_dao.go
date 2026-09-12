package dao

import (
	"context"
	"time"

	"github.com/yi-nology/git-sync-service/sync/model"
	"gorm.io/gorm"
)

type SyncRunStepDAO struct {
	db *gorm.DB
}

func NewSyncRunStepDAO(db *gorm.DB) *SyncRunStepDAO {
	return &SyncRunStepDAO{db: db}
}

func (d *SyncRunStepDAO) Create(step *model.SyncRunStep) error {
	return d.db.Create(step).Error
}

func (d *SyncRunStepDAO) Update(step *model.SyncRunStep) error {
	return d.db.Save(step).Error
}

func (d *SyncRunStepDAO) FindByRunID(runID uint) ([]*model.SyncRunStep, error) {
	var steps []*model.SyncRunStep
	err := d.db.Where("run_id = ?", runID).Order("id ASC").Find(&steps).Error
	return steps, err
}

func (d *SyncRunStepDAO) CleanupOlderThan(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	var total int64
	for {
		if ctx.Err() != nil {
			return total, ctx.Err()
		}
		result := d.db.WithContext(ctx).Where("created_at < ?", cutoff).Limit(1000).Delete(&model.SyncRunStep{})
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
