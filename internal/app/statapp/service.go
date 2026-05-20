package statapp

import (
	"context"
	"time"

	"github.com/jd/flashlink/internal/domain/link"
	"golang.org/x/sync/errgroup"
)

type Service struct {
	repo link.StatsRepository
	now  func() time.Time
}

func NewService(repo link.StatsRepository) *Service {
	return &Service{
		repo: repo,
		now:  time.Now,
	}
}

func (s *Service) GetLinkStats(ctx context.Context, code string) (link.LinkStats, error) {
	if err := link.ValidateShortCode(code); err != nil {
		return link.LinkStats{}, err
	}

	var stats link.LinkStats
	stats.Code = code

	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		pv, err := s.repo.CountVisits(ctx, code)
		stats.PV = pv
		return err
	})
	group.Go(func() error {
		uv, err := s.repo.CountUniqueVisitors(ctx, code)
		stats.UV = uv
		return err
	})
	group.Go(func() error {
		today, err := s.repo.CountTodayVisits(ctx, code)
		stats.TodayPV = today
		return err
	})
	group.Go(func() error {
		referers, err := s.repo.TopReferers(ctx, code, 10)
		stats.Referers = referers
		return err
	})

	if err := group.Wait(); err != nil {
		return link.LinkStats{}, err
	}
	stats.UpdatedAt = s.now()
	return stats, nil
}
