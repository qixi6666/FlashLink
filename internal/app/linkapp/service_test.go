package linkapp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jd/flashlink/internal/domain/link"
)

func TestCreateShortLink(t *testing.T) {
	t.Parallel()

	repo := &fakeRepo{}
	local := newFakeCache()
	remote := newFakeCache()
	filter := newFakeFilter(true)
	svc := New(Options{
		Repository: repo,
		IDs:        &fixedID{next: 31},
		LocalCache: local,
		RedisCache: remote,
		Filter:     filter,
		Domain:     "http://sho.rt",
	})
	svc.now = func() time.Time { return time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC) }

	got, err := svc.CreateShortLink(context.Background(), CreateRequest{LongURL: "https://example.com/p/31"})
	if err != nil {
		t.Fatalf("CreateShortLink returned error: %v", err)
	}
	if got.Code != link.NewShortCode(31) {
		t.Fatalf("code = %q, want %q", got.Code, link.NewShortCode(31))
	}
	if got.ShortURL != "http://sho.rt/"+got.Code {
		t.Fatalf("short url = %q", got.ShortURL)
	}
	if repo.creates != 1 {
		t.Fatalf("repo creates = %d, want 1", repo.creates)
	}
	if !filter.added[got.Code] {
		t.Fatalf("filter did not record code %q", got.Code)
	}
	if _, ok, _ := remote.Get(context.Background(), got.Code); !ok {
		t.Fatalf("remote cache miss for created code")
	}
}

func TestCreateShortLinkRejectsInvalidURL(t *testing.T) {
	t.Parallel()

	svc := New(Options{
		Repository: &fakeRepo{},
		IDs:        &fixedID{next: 1},
	})

	_, err := svc.CreateShortLink(context.Background(), CreateRequest{LongURL: "javascript:alert(1)"})
	if !errors.Is(err, link.ErrInvalidURL) {
		t.Fatalf("error = %v, want ErrInvalidURL", err)
	}
}

func TestCreateShortLinkWritesRepositoryBeforeFilterAndCaches(t *testing.T) {
	t.Parallel()

	events := &eventLog{}
	repo := &orderedRepo{events: events}
	remote := &orderedCache{events: events, name: "remote"}
	filter := &orderedFilter{events: events}
	svc := New(Options{
		Repository: repo,
		IDs:        &fixedID{next: 5},
		RedisCache: remote,
		Filter:     filter,
		Domain:     "http://sho.rt",
	})

	if _, err := svc.CreateShortLink(context.Background(), CreateRequest{LongURL: "https://example.com/p/5"}); err != nil {
		t.Fatalf("CreateShortLink returned error: %v", err)
	}

	want := []string{"repo.create", "filter.add", "cache.remote.set"}
	if got := events.snapshot(); !equalStrings(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func TestCreateShortLinkIgnoresRedisWriteError(t *testing.T) {
	t.Parallel()

	redisErr := errors.New("redis unavailable")
	repo := &fakeRepo{}
	filter := newFakeFilter(true)
	svc := New(Options{
		Repository: repo,
		IDs:        &fixedID{next: 6},
		RedisCache: &failingCache{err: redisErr},
		Filter:     filter,
		Domain:     "http://sho.rt",
	})

	got, err := svc.CreateShortLink(context.Background(), CreateRequest{LongURL: "https://example.com/p/6"})
	if err != nil {
		t.Fatalf("CreateShortLink returned error: %v", err)
	}
	if repo.creates != 1 {
		t.Fatalf("repo creates = %d, want 1", repo.creates)
	}
	if !filter.added[got.Code] {
		t.Fatalf("filter did not record code %q", got.Code)
	}
}

func TestCreateShortLinkReturnsFilterWriteError(t *testing.T) {
	t.Parallel()

	filterErr := errors.New("filter unavailable")
	repo := &fakeRepo{}
	local := newFakeCache()
	remote := newFakeCache()
	svc := New(Options{
		Repository: repo,
		IDs:        &fixedID{next: 7},
		LocalCache: local,
		RedisCache: remote,
		Filter:     &failingFilter{err: filterErr},
		Domain:     "http://sho.rt",
	})

	code := link.NewShortCode(7)
	_, err := svc.CreateShortLink(context.Background(), CreateRequest{LongURL: "https://example.com/p/7"})
	if !errors.Is(err, filterErr) {
		t.Fatalf("CreateShortLink error = %v, want filter error", err)
	}
	if repo.creates != 1 {
		t.Fatalf("repo creates = %d, want 1", repo.creates)
	}
	if _, ok, _ := remote.Get(context.Background(), code); ok {
		t.Fatalf("remote cache was written after filter failure")
	}
	if _, ok, _ := local.Get(context.Background(), code); ok {
		t.Fatalf("local cache was written after filter failure")
	}
}

func TestResolveUsesFilter(t *testing.T) {
	t.Parallel()

	repo := &fakeRepo{}
	svc := New(Options{
		Repository: repo,
		IDs:        &fixedID{next: 1},
		Filter:     newFakeFilter(false),
	})

	_, err := svc.Resolve(context.Background(), link.NewShortCode(1))
	if !errors.Is(err, link.ErrNotFound) {
		t.Fatalf("Resolve error = %v, want ErrNotFound", err)
	}
	if repo.finds != 0 {
		t.Fatalf("repo finds = %d, want 0", repo.finds)
	}
}

func TestResolveUsesLocalCache(t *testing.T) {
	t.Parallel()

	code := link.NewShortCode(2)
	local := newFakeCache()
	item := activeLink(2)
	_ = local.Set(context.Background(), item, time.Minute)
	repo := &fakeRepo{items: map[string]link.ShortLink{code: item}}

	svc := New(Options{
		Repository: repo,
		LocalCache: local,
		Filter:     newFakeFilter(true),
	})

	got, err := svc.Resolve(context.Background(), code)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if got.Code != code {
		t.Fatalf("code = %q, want %q", got.Code, code)
	}
	if repo.finds != 0 {
		t.Fatalf("repo finds = %d, want 0", repo.finds)
	}
}

func TestResolveLoadsFromRepositoryOnce(t *testing.T) {
	t.Parallel()

	code := link.NewShortCode(3)
	repo := &fakeRepo{
		items: map[string]link.ShortLink{code: activeLink(3)},
		block: make(chan struct{}),
	}
	local := newFakeCache()
	remote := newFakeCache()
	svc := New(Options{
		Repository: repo,
		LocalCache: local,
		RedisCache: remote,
		Filter:     newFakeFilter(true),
	})

	const callers = 8
	errs := make(chan error, callers)
	started := make(chan struct{})
	go func() {
		close(started)
		for i := 0; i < callers; i++ {
			_, err := svc.Resolve(context.Background(), code)
			errs <- err
		}
	}()
	<-started
	close(repo.block)

	for i := 0; i < callers; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("Resolve returned error: %v", err)
		}
	}
	if repo.finds != 1 {
		t.Fatalf("repo finds = %d, want 1", repo.finds)
	}
	if _, ok, _ := remote.Get(context.Background(), code); !ok {
		t.Fatalf("remote cache was not filled")
	}
}

func TestResolveDoesNotLazyDeleteFromExpiredCacheOnly(t *testing.T) {
	t.Parallel()

	code := link.NewShortCode(4)
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	expiredAt := now.Add(-time.Minute)
	expiredCached := activeLink(4)
	expiredCached.ExpireAt = &expiredAt
	activeFromDB := activeLink(4)
	activeFromDB.ExpireAt = nil

	local := newFakeCache()
	remote := newFakeCache()
	_ = local.Set(context.Background(), expiredCached, time.Minute)
	repo := &fakeRepo{items: map[string]link.ShortLink{code: activeFromDB}}
	cleaner := NewLazyExpiredCleaner(LazyExpiredCleanerOptions{
		Repository: &fakeLazyExpiredRepo{},
		QueueSize:  1,
	})
	svc := New(Options{
		Repository: repo,
		LocalCache: local,
		RedisCache: remote,
		Filter:     newFakeFilter(true),
		Cleaner:    cleaner,
	})
	svc.now = func() time.Time { return now }

	got, err := svc.Resolve(context.Background(), code)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if got.Code != code {
		t.Fatalf("code = %q, want %q", got.Code, code)
	}
	if len(cleaner.queue) != 0 {
		t.Fatalf("lazy cleaner queue length = %d, want 0", len(cleaner.queue))
	}
	if _, ok, _ := local.Get(context.Background(), code); !ok {
		t.Fatalf("local cache was not refreshed from db")
	}
}

type fixedID struct {
	next uint64
}

func (g *fixedID) NextID() uint64 {
	return g.next
}

type fakeRepo struct {
	mu      sync.Mutex
	items   map[string]link.ShortLink
	creates int
	finds   int
	block   chan struct{}
}

func (r *fakeRepo) Create(_ context.Context, item link.ShortLink) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.items == nil {
		r.items = make(map[string]link.ShortLink)
	}
	r.items[item.Code] = item
	r.creates++
	return nil
}

func (r *fakeRepo) FindByCode(_ context.Context, code string) (link.ShortLink, error) {
	r.mu.Lock()
	r.finds++
	block := r.block
	r.mu.Unlock()

	if block != nil {
		<-block
		r.mu.Lock()
		r.block = nil
		r.mu.Unlock()
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.items[code]
	if !ok {
		return link.ShortLink{}, link.ErrNotFound
	}
	return item, nil
}

func (r *fakeRepo) FindActiveByCode(ctx context.Context, code string) (link.ShortLink, error) {
	item, err := r.FindByCode(ctx, code)
	if err != nil {
		return link.ShortLink{}, err
	}
	if !item.IsActive(time.Now()) {
		return link.ShortLink{}, link.ErrExpired
	}
	return item, nil
}

type fakeCache struct {
	mu    sync.Mutex
	items map[string]link.ShortLink
}

func newFakeCache() *fakeCache {
	return &fakeCache{items: make(map[string]link.ShortLink)}
}

func (c *fakeCache) Get(_ context.Context, code string) (link.ShortLink, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	item, ok := c.items[code]
	return item, ok, nil
}

func (c *fakeCache) Set(_ context.Context, item link.ShortLink, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[item.Code] = item
	return nil
}

func (c *fakeCache) Delete(_ context.Context, codes []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, code := range codes {
		delete(c.items, code)
	}
	return nil
}

type fakeFilter struct {
	mu      sync.Mutex
	allow   bool
	added   map[string]bool
	checked int
}

func newFakeFilter(allow bool) *fakeFilter {
	return &fakeFilter{
		allow: allow,
		added: make(map[string]bool),
	}
}

func (f *fakeFilter) Add(_ context.Context, code string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.added[code] = true
	return nil
}

func (f *fakeFilter) MightContain(_ context.Context, code string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checked++
	return f.allow || f.added[code], nil
}

type eventLog struct {
	mu     sync.Mutex
	events []string
}

func (l *eventLog) add(event string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, event)
}

func (l *eventLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.events...)
}

type orderedRepo struct {
	events *eventLog
	item   link.ShortLink
}

func (r *orderedRepo) Create(_ context.Context, item link.ShortLink) error {
	r.events.add("repo.create")
	r.item = item
	return nil
}

func (r *orderedRepo) FindByCode(_ context.Context, code string) (link.ShortLink, error) {
	if r.item.Code != code {
		return link.ShortLink{}, link.ErrNotFound
	}
	return r.item, nil
}

func (r *orderedRepo) FindActiveByCode(ctx context.Context, code string) (link.ShortLink, error) {
	return r.FindByCode(ctx, code)
}

type orderedCache struct {
	events *eventLog
	name   string
	item   link.ShortLink
}

func (c *orderedCache) Get(_ context.Context, code string) (link.ShortLink, bool, error) {
	if c.item.Code != code {
		return link.ShortLink{}, false, nil
	}
	return c.item, true, nil
}

func (c *orderedCache) Set(_ context.Context, item link.ShortLink, _ time.Duration) error {
	c.events.add("cache." + c.name + ".set")
	c.item = item
	return nil
}

type failingCache struct {
	err error
}

func (c *failingCache) Get(context.Context, string) (link.ShortLink, bool, error) {
	return link.ShortLink{}, false, nil
}

func (c *failingCache) Set(context.Context, link.ShortLink, time.Duration) error {
	return c.err
}

type failingFilter struct {
	err error
}

func (f *failingFilter) Add(context.Context, string) error {
	return f.err
}

func (f *failingFilter) MightContain(context.Context, string) (bool, error) {
	return false, f.err
}

type orderedFilter struct {
	events *eventLog
}

func (f *orderedFilter) Add(context.Context, string) error {
	f.events.add("filter.add")
	return nil
}

func (f *orderedFilter) MightContain(context.Context, string) (bool, error) {
	return true, nil
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func activeLink(id uint64) link.ShortLink {
	return link.ShortLink{
		ID:        id,
		Code:      link.NewShortCode(id),
		LongURL:   "https://example.com/p",
		Domain:    "http://sho.rt",
		Status:    link.ShortLinkStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}
