package linkapp

import (
	"context"
	"time"

	"github.com/jd/flashlink/internal/domain/link"
)

type IDGenerator interface {
	NextID() uint64
}

type LinkCache interface {
	Get(ctx context.Context, code string) (link.ShortLink, bool, error)
	Set(ctx context.Context, item link.ShortLink, ttl time.Duration) error
}

type ExistenceFilter interface {
	Add(ctx context.Context, code string) error
	MightContain(ctx context.Context, code string) (bool, error)
}
