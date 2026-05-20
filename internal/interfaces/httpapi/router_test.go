package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jd/flashlink/internal/app/linkapp"
	"github.com/jd/flashlink/internal/app/statapp"
	"github.com/jd/flashlink/internal/domain/link"
)

func TestCreateShortLinkHandler(t *testing.T) {
	t.Parallel()

	repo := newHTTPFakeRepo()
	service := linkapp.New(linkapp.Options{
		Repository: repo,
		IDs:        fixedHTTPID(7),
		Filter:     alwaysHTTPFilter(true),
		Domain:     "http://sho.rt",
	})
	router := NewRouter(RouterOptions{Links: service})

	req := httptest.NewRequest(http.MethodPost, "/api/links", bytes.NewBufferString(`{"long_url":"https://example.com/a"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), link.NewShortCode(7)) {
		t.Fatalf("response does not contain code: %s", rec.Body.String())
	}
}

func TestRedirectHandler(t *testing.T) {
	t.Parallel()

	code := link.NewShortCode(9)
	repo := newHTTPFakeRepo()
	repo.items[code] = link.ShortLink{
		ID:      9,
		Code:    code,
		LongURL: "https://example.com/target",
		Status:  link.ShortLinkStatusActive,
	}
	service := linkapp.New(linkapp.Options{
		Repository: repo,
		Filter:     alwaysHTTPFilter(true),
	})
	router := NewRouter(RouterOptions{Links: service})

	req := httptest.NewRequest(http.MethodGet, "/"+code, nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "https://example.com/target" {
		t.Fatalf("Location = %q", got)
	}
}

func TestRedirectHandlerMissingCode(t *testing.T) {
	t.Parallel()

	service := linkapp.New(linkapp.Options{
		Repository: newHTTPFakeRepo(),
		Filter:     alwaysHTTPFilter(false),
	})
	router := NewRouter(RouterOptions{Links: service})

	req := httptest.NewRequest(http.MethodGet, "/"+link.NewShortCode(100), nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestStatsHandler(t *testing.T) {
	t.Parallel()

	code := link.NewShortCode(11)
	router := NewRouter(RouterOptions{
		Links: newHTTPLinkService(newHTTPFakeRepo()),
		Stats: statapp.NewService(httpStatsRepo{
			pv:      20,
			uv:      4,
			todayPV: 7,
		}),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/links/"+code+"/stats", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"pv":20`) {
		t.Fatalf("response missing pv: %s", rec.Body.String())
	}
}

type fixedHTTPID uint64

func (id fixedHTTPID) NextID() uint64 {
	return uint64(id)
}

type alwaysHTTPFilter bool

func (f alwaysHTTPFilter) Add(context.Context, string) error {
	return nil
}

func (f alwaysHTTPFilter) MightContain(context.Context, string) (bool, error) {
	return bool(f), nil
}

type httpFakeRepo struct {
	mu    sync.Mutex
	items map[string]link.ShortLink
}

func newHTTPLinkService(repo *httpFakeRepo) *linkapp.Service {
	return linkapp.New(linkapp.Options{
		Repository: repo,
		IDs:        fixedHTTPID(1),
		Filter:     alwaysHTTPFilter(true),
	})
}

func newHTTPFakeRepo() *httpFakeRepo {
	return &httpFakeRepo{items: make(map[string]link.ShortLink)}
}

func (r *httpFakeRepo) Create(_ context.Context, item link.ShortLink) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[item.Code] = item
	return nil
}

func (r *httpFakeRepo) FindByCode(_ context.Context, code string) (link.ShortLink, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.items[code]
	if !ok {
		return link.ShortLink{}, link.ErrNotFound
	}
	return item, nil
}

func (r *httpFakeRepo) FindActiveByCode(ctx context.Context, code string) (link.ShortLink, error) {
	item, err := r.FindByCode(ctx, code)
	if err != nil {
		return link.ShortLink{}, err
	}
	return item, nil
}

type httpStatsRepo struct {
	pv      uint64
	uv      uint64
	todayPV uint64
}

func (r httpStatsRepo) CountVisits(context.Context, string) (uint64, error) {
	return r.pv, nil
}

func (r httpStatsRepo) CountUniqueVisitors(context.Context, string) (uint64, error) {
	return r.uv, nil
}

func (r httpStatsRepo) CountTodayVisits(context.Context, string) (uint64, error) {
	return r.todayPV, nil
}

func (r httpStatsRepo) TopReferers(context.Context, string, int) ([]link.RefererStat, error) {
	return []link.RefererStat{{Referer: "https://ref.example", PV: 3}}, nil
}
