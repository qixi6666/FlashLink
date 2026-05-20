package statapp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jd/flashlink/internal/domain/link"
)

func TestRecorderFlushesBatch(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repo := &fakeVisitRepo{}
	recorder := NewRecorder(RecorderOptions{
		Repository:    repo,
		IDs:           &seqID{},
		QueueSize:     4,
		BatchSize:     2,
		FlushInterval: time.Hour,
		Now: func() time.Time {
			return time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
		},
	})
	recorder.Start(ctx)

	if err := recorder.Record(ctx, VisitEvent{Code: link.NewShortCode(1), IP: "127.0.0.1"}); err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	if err := recorder.Record(ctx, VisitEvent{Code: link.NewShortCode(1), IP: "127.0.0.2"}); err != nil {
		t.Fatalf("Record returned error: %v", err)
	}

	waitFor(t, func() bool {
		return repo.count() == 2
	})
}

func TestRecorderDrainsOnCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	repo := &fakeVisitRepo{}
	recorder := NewRecorder(RecorderOptions{
		Repository:    repo,
		IDs:           &seqID{},
		QueueSize:     4,
		BatchSize:     10,
		FlushInterval: time.Hour,
	})
	recorder.Start(ctx)

	if err := recorder.Record(ctx, VisitEvent{Code: link.NewShortCode(2)}); err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	cancel()

	select {
	case <-recorder.Done():
	case <-time.After(time.Second):
		t.Fatal("recorder did not stop")
	}
	if repo.count() != 1 {
		t.Fatalf("visit count = %d, want 1", repo.count())
	}
}

func TestRecorderQueueFull(t *testing.T) {
	t.Parallel()

	recorder := NewRecorder(RecorderOptions{
		Repository:    &fakeVisitRepo{},
		IDs:           &seqID{},
		QueueSize:     1,
		BatchSize:     10,
		FlushInterval: time.Hour,
	})

	if err := recorder.Record(context.Background(), VisitEvent{Code: link.NewShortCode(3)}); err != nil {
		t.Fatalf("first Record returned error: %v", err)
	}
	err := recorder.Record(context.Background(), VisitEvent{Code: link.NewShortCode(3)})
	if !errors.Is(err, link.ErrQueueFull) {
		t.Fatalf("second Record error = %v, want ErrQueueFull", err)
	}
}

type seqID struct {
	mu   sync.Mutex
	next uint64
}

func (s *seqID) NextID() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	return s.next
}

type fakeVisitRepo struct {
	mu     sync.Mutex
	visits []link.VisitLog
}

func (r *fakeVisitRepo) CreateBatch(_ context.Context, visits []link.VisitLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.visits = append(r.visits, visits...)
	return nil
}

func (r *fakeVisitRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.visits)
}

func waitFor(t *testing.T, ok func() bool) {
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
