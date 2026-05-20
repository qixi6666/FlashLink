package statapp

import (
	"context"
	"sync"
	"time"

	"github.com/jd/flashlink/internal/app/linkapp"
	"github.com/jd/flashlink/internal/domain/link"
)

const (
	defaultQueueSize     = 4096
	defaultBatchSize     = 200
	defaultFlushInterval = time.Second
)

type VisitEvent struct {
	Code      string
	IP        string
	UserAgent string
	Referer   string
	VisitedAt time.Time
}

type RecorderOptions struct {
	Repository    link.VisitLogRepository
	IDs           linkapp.IDGenerator
	QueueSize     int
	BatchSize     int
	FlushInterval time.Duration
	Now           func() time.Time
}

type Recorder struct {
	repo          link.VisitLogRepository
	ids           linkapp.IDGenerator
	queue         chan VisitEvent
	batchSize     int
	flushInterval time.Duration
	now           func() time.Time

	once sync.Once
	done chan struct{}
}

func NewRecorder(options RecorderOptions) *Recorder {
	queueSize := options.QueueSize
	if queueSize <= 0 {
		queueSize = defaultQueueSize
	}
	batchSize := options.BatchSize
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	flushInterval := options.FlushInterval
	if flushInterval <= 0 {
		flushInterval = defaultFlushInterval
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}

	return &Recorder{
		repo:          options.Repository,
		ids:           options.IDs,
		queue:         make(chan VisitEvent, queueSize),
		batchSize:     batchSize,
		flushInterval: flushInterval,
		now:           now,
		done:          make(chan struct{}),
	}
}

func (r *Recorder) Start(ctx context.Context) {
	r.once.Do(func() {
		go r.run(ctx)
	})
}

func (r *Recorder) Done() <-chan struct{} {
	return r.done
}

func (r *Recorder) Record(ctx context.Context, event VisitEvent) error {
	if event.VisitedAt.IsZero() {
		event.VisitedAt = r.now()
	}

	select {
	case r.queue <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return link.ErrQueueFull
	}
}

func (r *Recorder) run(ctx context.Context) {
	defer close(r.done)

	ticker := time.NewTicker(r.flushInterval)
	defer ticker.Stop()

	batch := make([]link.VisitLog, 0, r.batchSize)
	flush := func() {
		if len(batch) == 0 || r.repo == nil {
			batch = batch[:0]
			return
		}
		_ = r.repo.CreateBatch(context.Background(), batch)
		batch = batch[:0]
	}

	for {
		select {
		case <-ctx.Done():
			for {
				select {
				case event := <-r.queue:
					batch = append(batch, r.toVisitLog(event))
					if len(batch) >= r.batchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		case event := <-r.queue:
			batch = append(batch, r.toVisitLog(event))
			if len(batch) >= r.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func (r *Recorder) toVisitLog(event VisitEvent) link.VisitLog {
	return link.VisitLog{
		ID:        r.ids.NextID(),
		Code:      event.Code,
		VisitedAt: event.VisitedAt,
		IP:        event.IP,
		UserAgent: event.UserAgent,
		Referer:   event.Referer,
	}
}
