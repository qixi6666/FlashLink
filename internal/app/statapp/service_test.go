package statapp

import (
	"context"
	"testing"
	"time"

	"github.com/jd/flashlink/internal/domain/link"
)

func TestGetLinkStats(t *testing.T) {
	t.Parallel()

	code := link.NewShortCode(10)
	service := NewService(&fakeStatsRepo{})
	service.now = func() time.Time {
		return time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	}

	stats, err := service.GetLinkStats(context.Background(), code)
	if err != nil {
		t.Fatalf("GetLinkStats returned error: %v", err)
	}
	if stats.Code != code || stats.PV != 12 || stats.UV != 3 || stats.TodayPV != 5 {
		t.Fatalf("stats = %#v", stats)
	}
	if len(stats.Referers) != 1 || stats.Referers[0].Referer != "https://ref.example" {
		t.Fatalf("referers = %#v", stats.Referers)
	}
}

type fakeStatsRepo struct{}

func (r *fakeStatsRepo) CountVisits(context.Context, string) (uint64, error) {
	return 12, nil
}

func (r *fakeStatsRepo) CountUniqueVisitors(context.Context, string) (uint64, error) {
	return 3, nil
}

func (r *fakeStatsRepo) CountTodayVisits(context.Context, string) (uint64, error) {
	return 5, nil
}

func (r *fakeStatsRepo) TopReferers(context.Context, string, int) ([]link.RefererStat, error) {
	return []link.RefererStat{{Referer: "https://ref.example", PV: 4}}, nil
}
