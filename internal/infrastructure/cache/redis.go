package cache

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jd/flashlink/internal/domain/link"
	"github.com/redis/go-redis/v9"
)

type Redis struct {
	client *redis.Client
}

func NewRedis(client *redis.Client) *Redis {
	return &Redis{client: client}
}

func (c *Redis) Get(ctx context.Context, code string) (link.ShortLink, bool, error) {
	raw, err := c.client.Get(ctx, LinkKey(code)).Bytes()
	if errors.Is(err, redis.Nil) {
		return link.ShortLink{}, false, nil
	}
	if err != nil {
		return link.ShortLink{}, false, err
	}

	var item link.ShortLink
	if err := json.Unmarshal(raw, &item); err != nil {
		return link.ShortLink{}, false, err
	}
	return item, true, nil
}

func (c *Redis) Set(ctx context.Context, item link.ShortLink, ttl time.Duration) error {
	raw, err := json.Marshal(item)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, LinkKey(item.Code), raw, ttl).Err()
}

func (c *Redis) Delete(ctx context.Context, codes []string) error {
	if len(codes) == 0 {
		return nil
	}

	keys := make([]string, 0, len(codes))
	for _, code := range codes {
		keys = append(keys, LinkKey(code))
	}
	return c.client.Del(ctx, keys...).Err()
}
