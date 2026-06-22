package linkapp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jd/flashlink/internal/domain/link"
)

func TestAsyncShortLinkWriterFlushesBatch(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repo := newFakeBatchRepo()
	writer := NewAsyncShortLinkWriter(ctx, AsyncWriterOptions{
		Repository:    repo,
		BatchWriter:   repo,
		QueueSize:     8,
		BatchSize:     3,
		Workers:       1,
		FlushInterval: time.Hour,
	})

	for i := uint64(1); i <= 3; i++ {
		if err := writer.Create(ctx, activeLink(i)); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
	}

	waitForLinkApp(t, func() bool {
		return repo.batchCount() == 1 && repo.itemCount() == 3
	})
}

func TestAsyncShortLinkWriterDrainsOnCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	repo := newFakeBatchRepo()
	writer := NewAsyncShortLinkWriter(ctx, AsyncWriterOptions{
		Repository:    repo,
		BatchWriter:   repo,
		QueueSize:     8,
		BatchSize:     10,
		Workers:       1,
		FlushInterval: time.Hour,
	})

	if err := writer.Create(context.Background(), activeLink(10)); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	cancel()
	writer.Wait()

	if repo.itemCount() != 1 {
		t.Fatalf("item count = %d, want 1", repo.itemCount())
	}
}

func TestAsyncShortLinkWriterCreateWaitsWhenQueueFull(t *testing.T) {
	t.Parallel()

	writer := &AsyncShortLinkWriter{
		queue: make(chan link.ShortLink, 1),
	}
	writer.queue <- activeLink(1)

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	err := writer.Create(ctx, activeLink(2))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Create error = %v, want DeadlineExceeded", err)
	}
}

func BenchmarkAsyncShortLinkWriterCreate(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	repo := newFakeBatchRepo()
	writer := NewAsyncShortLinkWriter(ctx, AsyncWriterOptions{
		Repository:    repo,
		BatchWriter:   repo,
		QueueSize:     8192,
		BatchSize:     256,
		Workers:       4,
		FlushInterval: time.Millisecond,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := writer.Create(ctx, activeLink(uint64(i+1))); err != nil {
			b.Fatalf("Create returned error: %v", err)
		}
	}
	b.StopTimer()

	cancel()
	writer.Wait()
}

type fakeBatchRepo struct {
	mu      sync.Mutex
	items   map[string]link.ShortLink
	batches int
}

func newFakeBatchRepo() *fakeBatchRepo {
	return &fakeBatchRepo{items: make(map[string]link.ShortLink)}
}

func (r *fakeBatchRepo) Create(_ context.Context, item link.ShortLink) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[item.Code] = item
	return nil
}

func (r *fakeBatchRepo) CreateBatch(_ context.Context, items []link.ShortLink) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, item := range items {
		r.items[item.Code] = item
	}
	r.batches++
	return nil
}

func (r *fakeBatchRepo) FindByCode(_ context.Context, code string) (link.ShortLink, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.items[code]
	if !ok {
		return link.ShortLink{}, link.ErrNotFound
	}
	return item, nil
}

func (r *fakeBatchRepo) FindActiveByCode(ctx context.Context, code string) (link.ShortLink, error) {
	return r.FindByCode(ctx, code)
}

func (r *fakeBatchRepo) batchCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.batches
}

func (r *fakeBatchRepo) itemCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.items)
}

func waitForLinkApp(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.After(time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("condition was not met")
		case <-tick.C:
			if ok() {
				return
			}
		}
	}
}
