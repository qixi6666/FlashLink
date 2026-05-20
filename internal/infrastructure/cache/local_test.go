package cache

import (
	"context"
	"testing"
	"time"

	"github.com/jd/flashlink/internal/domain/link"
)

func TestLocalCacheExpiresItems(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	cache := NewLocalWithMaxEntries(2)
	cache.now = func() time.Time { return now }

	item := link.ShortLink{Code: link.NewShortCode(1), Status: link.ShortLinkStatusActive}
	if err := cache.Set(context.Background(), item, time.Second); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	cache.now = func() time.Time { return now.Add(2 * time.Second) }

	if _, ok, err := cache.Get(context.Background(), item.Code); err != nil || ok {
		t.Fatalf("Get = ok %v err %v, want expired miss", ok, err)
	}
}

func TestLocalCacheEvictsAtCapacity(t *testing.T) {
	t.Parallel()

	cache := NewLocalWithMaxEntries(1)
	first := link.ShortLink{Code: link.NewShortCode(1), Status: link.ShortLinkStatusActive}
	second := link.ShortLink{Code: link.NewShortCode(2), Status: link.ShortLinkStatusActive}

	_ = cache.Set(context.Background(), first, time.Minute)
	_ = cache.Set(context.Background(), second, time.Minute)

	if len(cache.items) != 1 {
		t.Fatalf("cache size = %d, want 1", len(cache.items))
	}
}
