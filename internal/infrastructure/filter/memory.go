package filter

import (
	"context"
	"sync"

	"github.com/jd/flashlink/internal/domain/link"
)

type Memory struct {
	mu    sync.RWMutex
	codes map[string]struct{}
}

func NewMemory() *Memory {
	return &Memory{codes: make(map[string]struct{})}
}

func (f *Memory) Add(_ context.Context, code string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.codes[code] = struct{}{}
	return nil
}

func (f *Memory) MightContain(_ context.Context, code string) (bool, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	_, ok := f.codes[code]
	return ok, nil
}

func (f *Memory) Rebuild(ctx context.Context, source link.ActiveCodeRepository, batchSize int) error {
	if batchSize <= 0 {
		batchSize = 1000
	}

	next := make(map[string]struct{})
	if err := source.ListActiveCodes(ctx, batchSize, func(codes []string) error {
		for _, code := range codes {
			next[code] = struct{}{}
		}
		return nil
	}); err != nil {
		return err
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.codes = next
	return nil
}
