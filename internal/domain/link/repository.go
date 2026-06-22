package link

import (
	"context"
	"time"
)

type ShortLinkRepository interface {
	Create(ctx context.Context, item ShortLink) error
	FindByCode(ctx context.Context, code string) (ShortLink, error)
	FindActiveByCode(ctx context.Context, code string) (ShortLink, error)
}

type ShortLinkBatchRepository interface {
	CreateBatch(ctx context.Context, items []ShortLink) error
}

type ActiveCodeRepository interface {
	ListActiveCodes(ctx context.Context, batchSize int, handle func([]string) error) error
}

type ExpiredShortLinkRepository interface {
	DeleteExpired(ctx context.Context, before time.Time, batchSize int, handleCodes func([]string) error) (int64, error)
}

type LazyExpiredShortLinkRepository interface {
	DeleteExpiredCode(ctx context.Context, code string, before time.Time) (bool, error)
}

type VisitLogRepository interface {
	CreateBatch(ctx context.Context, visits []VisitLog) error
}

type VisitLogCleanupRepository interface {
	DeleteVisitsBefore(ctx context.Context, before time.Time, limit int) (int64, error)
	DeleteDailyStatsBefore(ctx context.Context, before time.Time, limit int) (int64, error)
}

type StatsRepository interface {
	CountVisits(ctx context.Context, code string) (uint64, error)
	CountUniqueVisitors(ctx context.Context, code string) (uint64, error)
	CountTodayVisits(ctx context.Context, code string) (uint64, error)
	TopReferers(ctx context.Context, code string, limit int) ([]RefererStat, error)
}
