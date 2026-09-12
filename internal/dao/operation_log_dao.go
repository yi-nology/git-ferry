package dao

import (
	"time"

	"github.com/yi-nology/git-sync-service/sync/model"
	"gorm.io/gorm"
)

// OperationLogFilter 审计日志过滤条件。
type OperationLogFilter struct {
	Search    string // LIKE resource / detail
	Action    string // 精确匹配
	Actor     string // 精确匹配
	StartDate string // YYYY-MM-DD（含，按 created_at 起算）
	EndDate   string // YYYY-MM-DD（含，按 created_at 止算）
}

type OperationLogDAO struct {
	db *gorm.DB
}

func NewOperationLogDAO(db *gorm.DB) *OperationLogDAO {
	return &OperationLogDAO{db: db}
}

// Create 写入一条审计日志。
func (d *OperationLogDAO) Create(log *model.OperationLog) error {
	return d.db.Create(log).Error
}

// List 按过滤条件分页查询审计日志，返回列表与总数。
func (d *OperationLogDAO) List(page Pagination, filter *OperationLogFilter) ([]*model.OperationLog, int64, error) {
	var logs []*model.OperationLog
	var total int64

	query := d.db.Model(&model.OperationLog{})

	if filter.Search != "" {
		like := "%" + EscapeLikePattern(filter.Search) + "%"
		query = query.Where("(resource LIKE ? OR detail LIKE ?)", like, like)
	}
	if filter.Action != "" {
		query = query.Where("action = ?", filter.Action)
	}
	if filter.Actor != "" {
		query = query.Where("actor = ?", filter.Actor)
	}
	if filter.StartDate != "" {
		query = query.Where("created_at >= ?", filter.StartDate+" 00:00:00")
	}
	if filter.EndDate != "" {
		query = query.Where("created_at <= ?", filter.EndDate+" 23:59:59")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Offset(page.Offset).Limit(page.Limit).Order("id DESC").Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

// CountSince 统计 created_at >= t 的记录数。
func (d *OperationLogDAO) CountSince(t time.Time) (int64, error) {
	var count int64
	err := d.db.Model(&model.OperationLog{}).Where("created_at >= ?", t).Count(&count).Error
	return count, err
}

// CountAll 统计全部审计日志数。
func (d *OperationLogDAO) CountAll() (int64, error) {
	var count int64
	err := d.db.Model(&model.OperationLog{}).Count(&count).Error
	return count, err
}

// StatsResult 审计日志统计结果。
type StatsResult struct {
	Today int64
	Week  int64
	Total int64
}

// StatsOnce 一次查询返回今日、近 7 天、总操作数(替代三次串行 COUNT)。
func (d *OperationLogDAO) StatsOnce(startOfToday time.Time) (*StatsResult, error) {
	var r StatsResult
	weekStart := startOfToday.AddDate(0, 0, -6)
	err := d.db.Model(&model.OperationLog{}).
		Select(
			"COUNT(*) AS total",
			"SUM(CASE WHEN created_at >= ? THEN 1 ELSE 0 END) AS today",
			"SUM(CASE WHEN created_at >= ? THEN 1 ELSE 0 END) AS week",
			startOfToday, weekStart,
		).
		Scan(&r).Error
	return &r, err
}
