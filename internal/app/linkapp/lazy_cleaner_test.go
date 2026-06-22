package linkapp

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestLazyExpiredCleanerDeletesQueuedCode(t *testing.T) {
	t.Parallel()

	repo := &fakeLazyExpiredRepo{deleted: make(chan string, 1)}
	cleaner := NewLazyExpiredCleaner(LazyExpiredCleanerOptions{
		Repository: repo,
		QueueSize:  1,
		Timeout:    time.Second,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cleaner.Start(ctx)

	if ok := cleaner.Enqueue("abc", time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)); !ok {
		t.Fatal("Enqueue returned false")
	}

	select {
	case code := <-repo.deleted:
		if code != "abc" {
			t.Fatalf("deleted code = %q, want abc", code)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for lazy delete")
	}
}

func TestLazyExpiredCleanerDropsWhenQueueFull(t *testing.T) {
	t.Parallel()

	cleaner := NewLazyExpiredCleaner(LazyExpiredCleanerOptions{
		Repository: &fakeLazyExpiredRepo{},
		QueueSize:  1,
	})

	if ok := cleaner.Enqueue("a", time.Now()); !ok {
		t.Fatal("first Enqueue returned false")
	}
	if ok := cleaner.Enqueue("b", time.Now()); ok {
		t.Fatal("second Enqueue returned true, want queue-full drop")
	}
}

type fakeLazyExpiredRepo struct {
	mu      sync.Mutex
	deleted chan string
}

func (r *fakeLazyExpiredRepo) DeleteExpiredCode(_ context.Context, code string, _ time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.deleted != nil {
		r.deleted <- code
	}
	return true, nil
}
