package cleanupapp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jd/flashlink/internal/domain/link"
)

func TestRunOnceCleansAndRebuilds(t *testing.T) {
	t.Parallel()

	links := &fakeLinks{expiredCodes: []string{"a", "b"}}
	visits := &fakeVisits{visitDeletes: []int64{2, 0}, statDeletes: []int64{1, 0}}
	cache := &fakeCacheInvalidator{}
	filter := &fakeFilterRebuilder{}
	service := New(Options{
		Links:          links,
		Visits:         visits,
		Cache:          cache,
		Filter:         filter,
		BatchSize:      2,
		VisitRetention: 24 * time.Hour,
		StatRetention:  48 * time.Hour,
		Now: func() time.Time {
			return time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
		},
	})

	report, err := service.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if report.ExpiredLinksDeleted != 2 || report.VisitLogsDeleted != 2 || report.DailyStatsDeleted != 1 || !report.FilterRebuilt {
		t.Fatalf("report = %#v", report)
	}
	if len(cache.deleted) != 2 {
		t.Fatalf("cache deleted = %#v", cache.deleted)
	}
	if !filter.rebuilt {
		t.Fatal("filter was not rebuilt")
	}
}

func TestRunOnceStopsOnCacheDeleteError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("cache delete failed")
	service := New(Options{
		Links: &fakeLinks{expiredCodes: []string{"a"}},
		Cache: &fakeCacheInvalidator{err: wantErr},
	})

	_, err := service.RunOnce(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("RunOnce error = %v, want %v", err, wantErr)
	}
}

type fakeLinks struct {
	expiredCodes []string
}

func (f *fakeLinks) DeleteExpired(_ context.Context, _ time.Time, _ int, handleCodes func([]string) error) (int64, error) {
	if len(f.expiredCodes) == 0 {
		return 0, nil
	}
	if err := handleCodes(f.expiredCodes); err != nil {
		return 0, err
	}
	return int64(len(f.expiredCodes)), nil
}

func (f *fakeLinks) ListActiveCodes(_ context.Context, _ int, handle func([]string) error) error {
	return handle([]string{"active"})
}

type fakeVisits struct {
	visitDeletes []int64
	statDeletes  []int64
}

func (f *fakeVisits) DeleteVisitsBefore(context.Context, time.Time, int) (int64, error) {
	deleted := f.visitDeletes[0]
	f.visitDeletes = f.visitDeletes[1:]
	return deleted, nil
}

func (f *fakeVisits) DeleteDailyStatsBefore(context.Context, time.Time, int) (int64, error) {
	deleted := f.statDeletes[0]
	f.statDeletes = f.statDeletes[1:]
	return deleted, nil
}

type fakeCacheInvalidator struct {
	deleted []string
	err     error
}

func (f *fakeCacheInvalidator) Delete(_ context.Context, codes []string) error {
	if f.err != nil {
		return f.err
	}
	f.deleted = append(f.deleted, codes...)
	return nil
}

type fakeFilterRebuilder struct {
	rebuilt bool
}

func (f *fakeFilterRebuilder) Rebuild(context.Context, link.ActiveCodeRepository, int) error {
	f.rebuilt = true
	return nil
}
