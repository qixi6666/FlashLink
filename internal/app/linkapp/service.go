package linkapp

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/jd/flashlink/internal/domain/link"
	"golang.org/x/sync/singleflight"
)

const defaultCacheTTL = 30 * time.Minute

type Service struct {
	repo      link.ShortLinkRepository
	ids       IDGenerator
	local     LinkCache
	remote    LinkCache
	filter    ExistenceFilter
	domain    string
	cacheTTL  time.Duration
	now       func() time.Time
	loadGroup singleflight.Group
}

type Options struct {
	Repository link.ShortLinkRepository
	IDs        IDGenerator
	LocalCache LinkCache
	RedisCache LinkCache
	Filter     ExistenceFilter
	Domain     string
	CacheTTL   time.Duration
}

type CreateRequest struct {
	LongURL  string
	Domain   string
	ExpireAt *time.Time
}

type CreateResponse struct {
	Code     string     `json:"code"`
	ShortURL string     `json:"short_url"`
	LongURL  string     `json:"long_url"`
	ExpireAt *time.Time `json:"expire_at,omitempty"`
}

func New(options Options) *Service {
	cacheTTL := options.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = defaultCacheTTL
	}

	return &Service{
		repo:     options.Repository,
		ids:      options.IDs,
		local:    options.LocalCache,
		remote:   options.RedisCache,
		filter:   options.Filter,
		domain:   strings.TrimRight(options.Domain, "/"),
		cacheTTL: cacheTTL,
		now:      time.Now,
	}
}

func (s *Service) CreateShortLink(ctx context.Context, req CreateRequest) (CreateResponse, error) {
	longURL := strings.TrimSpace(req.LongURL)
	if !isValidLongURL(longURL) {
		return CreateResponse{}, link.ErrInvalidURL
	}

	domain := strings.TrimRight(strings.TrimSpace(req.Domain), "/")
	if domain == "" {
		domain = s.domain
	}

	now := s.now()
	id := s.ids.NextID()
	code := link.NewShortCode(id)
	item := link.ShortLink{
		ID:        id,
		Code:      code,
		LongURL:   longURL,
		Domain:    domain,
		ExpireAt:  req.ExpireAt,
		Status:    link.ShortLinkStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.repo.Create(ctx, item); err != nil {
		return CreateResponse{}, err
	}
	s.setCaches(ctx, item)
	if s.filter != nil {
		if err := s.filter.Add(ctx, code); err != nil {
			return CreateResponse{}, err
		}
	}

	return CreateResponse{
		Code:     code,
		ShortURL: domain + "/" + code,
		LongURL:  longURL,
		ExpireAt: req.ExpireAt,
	}, nil
}

func (s *Service) Resolve(ctx context.Context, code string) (link.ShortLink, error) {
	if err := link.ValidateShortCode(code); err != nil {
		return link.ShortLink{}, err
	}

	if s.filter != nil {
		ok, err := s.filter.MightContain(ctx, code)
		if err == nil && !ok {
			return link.ShortLink{}, link.ErrNotFound
		}
	}

	if item, ok, err := s.getCache(ctx, s.local, code); err != nil {
		// Local cache failures should not block redirect; fall through to lower layers.
	} else if ok {
		return item, nil
	}

	loaded, err, _ := s.loadGroup.Do(code, func() (any, error) {
		if item, ok, err := s.getCache(ctx, s.remote, code); err != nil {
			// Redis cache errors degrade to MySQL lookup.
		} else if ok {
			s.setCache(ctx, s.local, item)
			return item, nil
		}

		item, err := s.repo.FindActiveByCode(ctx, code)
		if err != nil {
			return link.ShortLink{}, err
		}
		s.setCaches(ctx, item)
		return item, nil
	})
	if err != nil {
		return link.ShortLink{}, err
	}

	return loaded.(link.ShortLink), nil
}

func (s *Service) getCache(ctx context.Context, cache LinkCache, code string) (link.ShortLink, bool, error) {
	if cache == nil {
		return link.ShortLink{}, false, nil
	}
	item, ok, err := cache.Get(ctx, code)
	if err != nil || !ok {
		return link.ShortLink{}, ok, err
	}
	if item.IsActive(s.now()) {
		return item, true, nil
	}
	return link.ShortLink{}, false, nil
}

func (s *Service) setCaches(ctx context.Context, item link.ShortLink) {
	s.setCache(ctx, s.local, item)
	s.setCache(ctx, s.remote, item)
}

func (s *Service) setCache(ctx context.Context, cache LinkCache, item link.ShortLink) {
	if cache == nil {
		return
	}

	ttl := s.cacheTTL
	if item.ExpireAt != nil {
		untilExpire := item.ExpireAt.Sub(s.now())
		if untilExpire <= 0 {
			return
		}
		if untilExpire < ttl {
			ttl = untilExpire
		}
	}
	_ = cache.Set(ctx, item, ttl)
}

func isValidLongURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
