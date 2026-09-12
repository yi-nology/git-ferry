package dao

import (
	"context"
	"time"

	"github.com/yi-nology/git-sync-service/sync/model"
	"gorm.io/gorm"
)

type SyncRunDAO struct {
	db *gorm.DB
}

func NewSyncRunDAO(db *gorm.DB) *SyncRunDAO {
	return &SyncRunDAO{db: db}
}

// DB 返回底层 gorm.DB,供跨 DAO 事务使用。
func (d *SyncRunDAO) DB() *gorm.DB {
	return d.db
}

// FindByTaskKey 列表查询,不加载 Steps(列表页不需要步骤详情,避免 N+1)。
func (d *SyncRunDAO) FindByTaskKey(taskKey string, page Pagination) ([]*model.SyncRun, int64, error) {
	var runs []*model.SyncRun
	query := d.db.Where("task_key = ?", taskKey)
	total, err := Paginate(query, page, &runs)
	return runs, total, err
}

// FindByIDWithSteps 单条详情查询,加载 Steps 子记录。
func (d *SyncRunDAO) FindByIDWithSteps(id uint) (*model.SyncRun, error) {
	var run model.SyncRun
	err := d.db.Preload("Steps").First(&run, id).Error
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (d *SyncRunDAO) Create(run *model.SyncRun) error {
	return d.db.Create(run).Error
}

func (d *SyncRunDAO) Update(run *model.SyncRun) error {
	return d.db.Save(run).Error
}

func (d *SyncRunDAO) Delete(id uint) error {
	return d.db.Delete(&model.SyncRun{}, id).Error
}

func (d *SyncRunDAO) FindRecent(page Pagination) ([]*model.SyncRun, error) {
	var runs []*model.SyncRun
	err := d.db.Offset(page.Offset).Limit(page.Limit).Order("id DESC").Find(&runs).Error
	return runs, err
}

func (d *SyncRunDAO) CleanupOlderThan(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	var total int64
	for {
		// 每轮检查 context 是否已取消,支持优雅关闭时中断清理
		if ctx.Err() != nil {
			return total, ctx.Err()
		}
		result := d.db.WithContext(ctx).Where("created_at < ?", cutoff).Limit(1000).Delete(&model.SyncRun{})
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

func (d *SyncRunDAO) CountByTaskKey(taskKey string) (int64, error) {
	var count int64
	err := d.db.Model(&model.SyncRun{}).Where("task_key = ?", taskKey).Count(&count).Error
	return count, err
}
