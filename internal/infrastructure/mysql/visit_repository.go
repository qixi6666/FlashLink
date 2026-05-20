package mysql

import (
	"context"
	"time"

	"github.com/jd/flashlink/internal/domain/link"
	"gorm.io/gorm"
)

type VisitRepository struct {
	db  *gorm.DB
	now func() time.Time
}

func NewVisitRepository(db *gorm.DB) *VisitRepository {
	return &VisitRepository{
		db:  db,
		now: time.Now,
	}
}

func (r *VisitRepository) CreateBatch(ctx context.Context, visits []link.VisitLog) error {
	if len(visits) == 0 {
		return nil
	}

	records := make([]VisitLogRecord, 0, len(visits))
	for _, visit := range visits {
		records = append(records, visitLogToRecord(visit))
	}

	return r.db.WithContext(ctx).Create(&records).Error
}

func (r *VisitRepository) CountVisits(ctx context.Context, code string) (uint64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&VisitLogRecord{}).
		Where("code = ?", code).
		Count(&count).
		Error
	return uint64(count), err
}

func (r *VisitRepository) CountUniqueVisitors(ctx context.Context, code string) (uint64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&VisitLogRecord{}).
		Where("code = ?", code).
		Distinct("ip").
		Count(&count).
		Error
	return uint64(count), err
}

func (r *VisitRepository) CountTodayVisits(ctx context.Context, code string) (uint64, error) {
	now := r.now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	end := start.AddDate(0, 0, 1)

	var count int64
	err := r.db.WithContext(ctx).
		Model(&VisitLogRecord{}).
		Where("code = ? AND visited_at >= ? AND visited_at < ?", code, start, end).
		Count(&count).
		Error
	return uint64(count), err
}

func (r *VisitRepository) TopReferers(ctx context.Context, code string, limit int) ([]link.RefererStat, error) {
	if limit <= 0 {
		limit = 10
	}

	var records []refererStatRecord
	err := r.db.WithContext(ctx).
		Model(&VisitLogRecord{}).
		Select("referer, COUNT(*) AS pv").
		Where("code = ? AND referer <> ''", code).
		Group("referer").
		Order("pv DESC").
		Limit(limit).
		Scan(&records).
		Error
	if err != nil {
		return nil, err
	}

	stats := make([]link.RefererStat, 0, len(records))
	for _, record := range records {
		stats = append(stats, link.RefererStat{
			Referer: record.Referer,
			PV:      record.PV,
		})
	}
	return stats, nil
}

func (r *VisitRepository) DeleteVisitsBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	if limit <= 0 {
		limit = 1000
	}

	result := r.db.WithContext(ctx).Exec("DELETE FROM visit_log WHERE visited_at < ? LIMIT ?", before, limit)
	return result.RowsAffected, result.Error
}

func (r *VisitRepository) DeleteDailyStatsBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	if limit <= 0 {
		limit = 1000
	}

	cutoffDate := time.Date(before.Year(), before.Month(), before.Day(), 0, 0, 0, 0, before.Location())
	result := r.db.WithContext(ctx).Exec("DELETE FROM link_stat_daily WHERE stat_date < ? LIMIT ?", cutoffDate, limit)
	return result.RowsAffected, result.Error
}

func visitLogToRecord(visit link.VisitLog) VisitLogRecord {
	return VisitLogRecord{
		ID:        visit.ID,
		Code:      visit.Code,
		VisitedAt: visit.VisitedAt,
		IP:        visit.IP,
		UserAgent: visit.UserAgent,
		Referer:   visit.Referer,
	}
}
