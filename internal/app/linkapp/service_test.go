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
	if _, ok, _ := local.Get(context.Background(), got.Code); !ok {
		t.Fatalf("local cache miss for created code")
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
