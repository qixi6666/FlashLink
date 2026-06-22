package cache

import (
	"container/list"
	"context"
	"sync"
	"time"

	"github.com/jd/flashlink/internal/domain/link"
)

type Local struct {
	mu         sync.RWMutex
	items      map[string]*list.Element
	lru        *list.List
	maxEntries int
	now        func() time.Time
}

type localItem struct {
	code     string
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
		items:      make(map[string]*list.Element),
		lru:        list.New(),
		maxEntries: maxEntries,
		now:        time.Now,
	}
}

func (c *Local) Get(_ context.Context, code string) (link.ShortLink, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	element, ok := c.items[code]
	if !ok {
		return link.ShortLink{}, false, nil
	}
	item := element.Value.(*localItem)
	if !item.expireAt.IsZero() && !item.expireAt.After(c.now()) {
		c.removeElementLocked(element)
		return link.ShortLink{}, false, nil
	}
	c.lru.MoveToFront(element)
	return item.value, true, nil
}

func (c *Local) Set(_ context.Context, item link.ShortLink, ttl time.Duration) error {
	expireAt := time.Time{}
	if ttl > 0 {
		expireAt = c.now().Add(ttl)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if element, ok := c.items[item.Code]; ok {
		entry := element.Value.(*localItem)
		entry.value = item
		entry.expireAt = expireAt
		c.lru.MoveToFront(element)
		return nil
	}

	if len(c.items) >= c.maxEntries {
		c.evictOneLocked()
	}
	c.items[item.Code] = c.lru.PushFront(&localItem{
		code:     item.Code,
		value:    item,
		expireAt: expireAt,
	})
	return nil
}

func (c *Local) Delete(_ context.Context, codes []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, code := range codes {
		element, ok := c.items[code]
		if !ok {
			continue
		}
		c.removeElementLocked(element)
	}
	return nil
}

func (c *Local) evictOneLocked() {
	now := c.now()
	for element := c.lru.Back(); element != nil; element = element.Prev() {
		item := element.Value.(*localItem)
		if !item.expireAt.IsZero() && !item.expireAt.After(now) {
			c.removeElementLocked(element)
			return
		}
	}
	if element := c.lru.Back(); element != nil {
		c.removeElementLocked(element)
	}
}

func (c *Local) removeElementLocked(element *list.Element) {
	item := element.Value.(*localItem)
	delete(c.items, item.code)
	c.lru.Remove(element)
}
