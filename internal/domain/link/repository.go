package link

import "context"

type ShortLinkRepository interface {
	Create(ctx context.Context, item ShortLink) error
	FindByCode(ctx context.Context, code string) (ShortLink, error)
	FindActiveByCode(ctx context.Context, code string) (ShortLink, error)
}
