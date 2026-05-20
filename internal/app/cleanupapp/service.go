package cleanupapp

import (
	"context"
	"time"

	"github.com/jd/flashlink/internal/domain/link"
)

const (
	defaultBatchSize      = 1000
	defaultVisitRetention = 30 * 24 * time.Hour
	defaultStatRetention  = 180 * 24 * time.Hour
)

type CacheInvalidator interface {
	Delete(ctx context.Context, codes []string) error
}

type FilterRebuilder interface {
	Rebuild(ctx context.Context, source link.ActiveCodeRepository, batchSize int) error
}

type Options struct {
	Links interface {
		link.ActiveCodeRepository
		link.ExpiredShortLinkRepository
	}
	Visits         link.VisitLogCleanupRepository
	Cache          CacheInvalidator
	Filter         FilterRebuilder
	BatchSize      int
	VisitRetention time.Duration
	StatRetention  time.Duration
	Now            func() time.Time
}

type Service struct {
	links interface {
		link.ActiveCodeRepository
		link.ExpiredShortLinkRepository
	}
	visits         link.VisitLogCleanupRepository
	cache          CacheInvalidator
	filter         FilterRebuilder
	batchSize      int
	visitRetention time.Duration
	statRetention  time.Duration
	now            func() time.Time
}

type Report struct {
	ExpiredLinksDeleted int64
	VisitLogsDeleted    int64
	DailyStatsDeleted   int64
	FilterRebuilt       bool
}

func New(options Options) *Service {
	batchSize := options.BatchSize
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	visitRetention := options.VisitRetention
	if visitRetention <= 0 {
		visitRetention = defaultVisitRetention
	}
	statRetention := options.StatRetention
	if statRetention <= 0 {
		statRetention = defaultStatRetention
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}

	return &Service{
		links:          options.Links,
		visits:         options.Visits,
		cache:          options.Cache,
		filter:         options.Filter,
		batchSize:      batchSize,
		visitRetention: visitRetention,
		statRetention:  statRetention,
		now:            now,
	}
}

func (s *Service) RunOnce(ctx context.Context) (Report, error) {
	var report Report
	now := s.now()

	if s.links != nil {
		deleted, err := s.links.DeleteExpired(ctx, now, s.batchSize, func(codes []string) error {
			if s.cache == nil {
				return nil
			}
			return s.cache.Delete(ctx, codes)
		})
		if err != nil {
			return report, err
		}
		report.ExpiredLinksDeleted = deleted
	}

	if s.visits != nil {
		deleted, err := deleteAll(ctx, s.batchSize, func() (int64, error) {
			return s.visits.DeleteVisitsBefore(ctx, now.Add(-s.visitRetention), s.batchSize)
		})
		if err != nil {
			return report, err
		}
		report.VisitLogsDeleted = deleted

		deleted, err = deleteAll(ctx, s.batchSize, func() (int64, error) {
			return s.visits.DeleteDailyStatsBefore(ctx, now.Add(-s.statRetention), s.batchSize)
		})
		if err != nil {
			return report, err
		}
		report.DailyStatsDeleted = deleted
	}

	if s.links != nil && s.filter != nil {
		if err := s.filter.Rebuild(ctx, s.links, s.batchSize); err != nil {
			return report, err
		}
		report.FilterRebuilt = true
	}

	return report, nil
}

func deleteAll(ctx context.Context, limit int, deleteBatch func() (int64, error)) (int64, error) {
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}

		deleted, err := deleteBatch()
		if err != nil {
			return total, err
		}
		total += deleted
		if deleted < int64(limit) {
			return total, nil
		}
	}
}
