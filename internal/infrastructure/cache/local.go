package cache

import (
	"context"
	"sync"
	"time"

	"github.com/jd/flashlink/internal/domain/link"
)

type Local struct {
	mu         sync.RWMutex
	items      map[string]localItem
	maxEntries int
	now        func() time.Time
}

type localItem struct {
	value    link.ShortLink
	expireAt time.Time
}

func NewLocal() *Local {
	return NewLocalWithMaxEntries(10000)
}

func NewLocalWithMaxEntries(maxEntries int) *Local {
	if maxEntries <= 0 {
		maxEntries = 10000
	}
	return &Local{
		items:      make(map[string]localItem),
		maxEntries: maxEntries,
		now:        time.Now,
	}
}

func (c *Local) Get(_ context.Context, code string) (link.ShortLink, bool, error) {
	c.mu.RLock()
	item, ok := c.items[code]
	c.mu.RUnlock()
	if !ok {
		return link.ShortLink{}, false, nil
	}
	if !item.expireAt.IsZero() && !item.expireAt.After(c.now()) {
		c.mu.Lock()
		delete(c.items, code)
		c.mu.Unlock()
		return link.ShortLink{}, false, nil
	}
	return item.value, true, nil
}

func (c *Local) Set(_ context.Context, item link.ShortLink, ttl time.Duration) error {
	expireAt := time.Time{}
	if ttl > 0 {
		expireAt = c.now().Add(ttl)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) >= c.maxEntries {
		c.evictOneLocked()
	}
	c.items[item.Code] = localItem{value: item, expireAt: expireAt}
	return nil
}

func (c *Local) evictOneLocked() {
	now := c.now()
	for code, item := range c.items {
		if !item.expireAt.IsZero() && !item.expireAt.After(now) {
			delete(c.items, code)
			return
		}
	}
	for code := range c.items {
		delete(c.items, code)
		return
	}
}
