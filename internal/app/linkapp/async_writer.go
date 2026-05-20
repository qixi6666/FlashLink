package linkapp

import (
	"context"
	"sync"
	"time"

	"github.com/jd/flashlink/internal/domain/link"
)

const (
	defaultWriteQueueSize     = 8192
	defaultWriteBatchSize     = 256
	defaultWriteWorkers       = 4
	defaultWriteFlushInterval = 10 * time.Millisecond
)

type AsyncWriterOptions struct {
	Repository    link.ShortLinkRepository
	BatchWriter   link.ShortLinkBatchRepository
	QueueSize     int
	BatchSize     int
	Workers       int
	FlushInterval time.Duration
}

type AsyncShortLinkWriter struct {
	repo          link.ShortLinkRepository
	batchWriter   link.ShortLinkBatchRepository
	ring          *shortLinkRing
	batchSize     int
	flushInterval time.Duration
	pool          sync.Pool
	wg            sync.WaitGroup
}

func NewAsyncShortLinkWriter(ctx context.Context, options AsyncWriterOptions) *AsyncShortLinkWriter {
	queueSize := options.QueueSize
	if queueSize <= 0 {
		queueSize = defaultWriteQueueSize
	}
	batchSize := options.BatchSize
	if batchSize <= 0 {
		batchSize = defaultWriteBatchSize
	}
	workers := options.Workers
	if workers <= 0 {
		workers = defaultWriteWorkers
	}
	flushInterval := options.FlushInterval
	if flushInterval <= 0 {
		flushInterval = defaultWriteFlushInterval
	}

	writer := &AsyncShortLinkWriter{
		repo:          options.Repository,
		batchWriter:   options.BatchWriter,
		ring:          newShortLinkRing(queueSize),
		batchSize:     batchSize,
		flushInterval: flushInterval,
	}
	writer.pool.New = func() any {
		batch := make([]link.ShortLink, 0, batchSize)
		return &batch
	}

	for i := 0; i < workers; i++ {
		writer.wg.Add(1)
		go writer.run(ctx)
	}

	return writer
}

func (w *AsyncShortLinkWriter) Create(ctx context.Context, item link.ShortLink) error {
	if err := link.ValidateShortCode(item.Code); err != nil {
		return err
	}
	return w.ring.Push(ctx, item)
}

func (w *AsyncShortLinkWriter) FindByCode(ctx context.Context, code string) (link.ShortLink, error) {
	return w.repo.FindByCode(ctx, code)
}

func (w *AsyncShortLinkWriter) FindActiveByCode(ctx context.Context, code string) (link.ShortLink, error) {
	return w.repo.FindActiveByCode(ctx, code)
}

func (w *AsyncShortLinkWriter) Wait() {
	w.wg.Wait()
}

func (w *AsyncShortLinkWriter) run(ctx context.Context) {
	defer w.wg.Done()

	for {
		batchPtr := w.pool.Get().(*[]link.ShortLink)
		batch := (*batchPtr)[:0]

		var err error
		batch, err = w.ring.PopBatch(ctx, batch, w.batchSize, w.flushInterval)
		if err != nil {
			w.drain(batch)
			*batchPtr = batch[:0]
			w.pool.Put(batchPtr)
			return
		}

		w.flush(batch)
		*batchPtr = batch[:0]
		w.pool.Put(batchPtr)
	}
}

func (w *AsyncShortLinkWriter) drain(batch []link.ShortLink) {
	if len(batch) > 0 {
		w.flush(batch)
	}

	for {
		batch = w.ring.TryPopBatch(batch[:0], w.batchSize)
		if len(batch) == 0 {
			return
		}
		w.flush(batch)
	}
}

func (w *AsyncShortLinkWriter) flush(batch []link.ShortLink) {
	if len(batch) == 0 {
		return
	}
	_ = w.batchWriter.CreateBatch(context.Background(), batch)
}

type shortLinkRing struct {
	mu        sync.Mutex
	items     []link.ShortLink
	head      int
	tail      int
	slots     chan struct{}
	available chan struct{}
}

func newShortLinkRing(capacity int) *shortLinkRing {
	r := &shortLinkRing{
		items:     make([]link.ShortLink, capacity),
		slots:     make(chan struct{}, capacity),
		available: make(chan struct{}, capacity),
	}
	for i := 0; i < capacity; i++ {
		r.slots <- struct{}{}
	}
	return r
}

func (r *shortLinkRing) Push(ctx context.Context, item link.ShortLink) error {
	select {
	case <-r.slots:
	case <-ctx.Done():
		return ctx.Err()
	}

	r.mu.Lock()
	r.items[r.tail] = item
	r.tail = (r.tail + 1) % len(r.items)
	r.mu.Unlock()

	r.available <- struct{}{}
	return nil
}

func (r *shortLinkRing) PopBatch(ctx context.Context, dst []link.ShortLink, max int, wait time.Duration) ([]link.ShortLink, error) {
	select {
	case <-r.available:
		dst = r.popAfterAvailable(dst)
	case <-ctx.Done():
		return dst, ctx.Err()
	}

	if len(dst) >= max {
		return dst, nil
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()

	for len(dst) < max {
		select {
		case <-r.available:
			dst = r.popAfterAvailable(dst)
		case <-timer.C:
			return dst, nil
		case <-ctx.Done():
			return dst, ctx.Err()
		}
	}
	return dst, nil
}

func (r *shortLinkRing) TryPopBatch(dst []link.ShortLink, max int) []link.ShortLink {
	for len(dst) < max {
		select {
		case <-r.available:
			dst = r.popAfterAvailable(dst)
		default:
			return dst
		}
	}
	return dst
}

func (r *shortLinkRing) popAfterAvailable(dst []link.ShortLink) []link.ShortLink {
	r.mu.Lock()
	item := r.items[r.head]
	r.items[r.head] = link.ShortLink{}
	r.head = (r.head + 1) % len(r.items)
	r.mu.Unlock()

	r.slots <- struct{}{}
	return append(dst, item)
}
