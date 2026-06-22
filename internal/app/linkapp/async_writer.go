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
	queue         chan link.ShortLink
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
		batchWriter:   resolveBatchWriter(options),
		queue:         make(chan link.ShortLink, queueSize),
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
	select {
	case w.queue <- item:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
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
		batch, err = w.popBatch(ctx, batch)
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

func (w *AsyncShortLinkWriter) popBatch(ctx context.Context, batch []link.ShortLink) ([]link.ShortLink, error) {
	select {
	case item := <-w.queue:
		batch = append(batch, item)
	case <-ctx.Done():
		return batch, ctx.Err()
	}

	if len(batch) >= w.batchSize {
		return batch, nil
	}

	timer := time.NewTimer(w.flushInterval)
	defer timer.Stop()

	for len(batch) < w.batchSize {
		select {
		case item := <-w.queue:
			batch = append(batch, item)
		case <-timer.C:
			return batch, nil
		case <-ctx.Done():
			return batch, ctx.Err()
		}
	}
	return batch, nil
}

func (w *AsyncShortLinkWriter) drain(batch []link.ShortLink) {
	if len(batch) > 0 {
		w.flush(batch)
	}

	for {
		batch = batch[:0]
		for len(batch) < w.batchSize {
			select {
			case item := <-w.queue:
				batch = append(batch, item)
			default:
				if len(batch) > 0 {
					w.flush(batch)
				}
				return
			}
		}
		w.flush(batch)
	}
}

func (w *AsyncShortLinkWriter) flush(batch []link.ShortLink) {
	if len(batch) == 0 {
		return
	}
	if w.batchWriter != nil {
		_ = w.batchWriter.CreateBatch(context.Background(), batch)
		return
	}
	for _, item := range batch {
		_ = w.repo.Create(context.Background(), item)
	}
}

func resolveBatchWriter(options AsyncWriterOptions) link.ShortLinkBatchRepository {
	if options.BatchWriter != nil {
		return options.BatchWriter
	}
	if batchWriter, ok := options.Repository.(link.ShortLinkBatchRepository); ok {
		return batchWriter
	}
	return nil
}
