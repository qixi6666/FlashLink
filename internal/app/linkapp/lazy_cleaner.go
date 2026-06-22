package linkapp

import (
	"context"
	"log"
	"time"

	"github.com/jd/flashlink/internal/domain/link"
)

const (
	defaultLazyCleanerQueueSize = 4096
	defaultLazyCleanerTimeout   = 500 * time.Millisecond
)

type LazyExpiredCleanerOptions struct {
	Repository link.LazyExpiredShortLinkRepository
	QueueSize  int
	Timeout    time.Duration
	Logger     *log.Logger
}

type LazyExpiredCleaner struct {
	repo    link.LazyExpiredShortLinkRepository
	queue   chan lazyExpiredItem
	timeout time.Duration
	logger  *log.Logger
}

type lazyExpiredItem struct {
	code   string
	before time.Time
}

func NewLazyExpiredCleaner(options LazyExpiredCleanerOptions) *LazyExpiredCleaner {
	queueSize := options.QueueSize
	if queueSize <= 0 {
		queueSize = defaultLazyCleanerQueueSize
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultLazyCleanerTimeout
	}
	logger := options.Logger
	if logger == nil {
		logger = log.Default()
	}

	return &LazyExpiredCleaner{
		repo:    options.Repository,
		queue:   make(chan lazyExpiredItem, queueSize),
		timeout: timeout,
		logger:  logger,
	}
}

func (c *LazyExpiredCleaner) Start(ctx context.Context) {
	if c == nil || c.repo == nil {
		return
	}
	go c.run(ctx)
}

func (c *LazyExpiredCleaner) Enqueue(code string, before time.Time) bool {
	if c == nil || c.repo == nil || code == "" {
		return false
	}

	select {
	case c.queue <- lazyExpiredItem{code: code, before: before}:
		return true
	default:
		return false
	}
}

func (c *LazyExpiredCleaner) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case item := <-c.queue:
			c.deleteOne(ctx, item)
		}
	}
}

func (c *LazyExpiredCleaner) deleteOne(parent context.Context, item lazyExpiredItem) {
	ctx, cancel := context.WithTimeout(parent, c.timeout)
	defer cancel()

	if _, err := c.repo.DeleteExpiredCode(ctx, item.code, item.before); err != nil && c.logger != nil {
		c.logger.Printf("lazy expired short link cleanup failed code=%s: %v", item.code, err)
	}
}
