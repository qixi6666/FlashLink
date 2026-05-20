package cleanupapp

import (
	"context"
	"log"
	"time"
)

type Scheduler struct {
	service  *Service
	interval time.Duration
	logger   *log.Logger
	done     chan struct{}
}

func NewScheduler(service *Service, interval time.Duration, logger *log.Logger) *Scheduler {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	if logger == nil {
		logger = log.Default()
	}
	return &Scheduler{
		service:  service,
		interval: interval,
		logger:   logger,
		done:     make(chan struct{}),
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	go s.run(ctx)
}

func (s *Scheduler) Done() <-chan struct{} {
	return s.done
}

func (s *Scheduler) run(ctx context.Context) {
	defer close(s.done)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			report, err := s.service.RunOnce(ctx)
			if err != nil {
				s.logger.Printf("cleanup failed: %v", err)
				continue
			}
			s.logger.Printf(
				"cleanup finished expired_links=%d visit_logs=%d daily_stats=%d filter_rebuilt=%t",
				report.ExpiredLinksDeleted,
				report.VisitLogsDeleted,
				report.DailyStatsDeleted,
				report.FilterRebuilt,
			)
		}
	}
}
