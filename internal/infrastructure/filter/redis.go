package filter

import (
	"context"

	"github.com/jd/flashlink/internal/domain/link"
	"github.com/redis/go-redis/v9"
)

const (
	redisFilterKey     = "flashlink:filter:codes"
	redisFilterTempKey = "flashlink:filter:codes:rebuild"
	redisFilterMarker  = "__flashlink_marker__"
)

type RedisSet struct {
	client *redis.Client
}

func NewRedisSet(client *redis.Client) *RedisSet {
	return &RedisSet{client: client}
}

func (f *RedisSet) Add(ctx context.Context, code string) error {
	return f.client.SAdd(ctx, redisFilterKey, code).Err()
}

func (f *RedisSet) MightContain(ctx context.Context, code string) (bool, error) {
	return f.client.SIsMember(ctx, redisFilterKey, code).Result()
}

func (f *RedisSet) Rebuild(ctx context.Context, source link.ActiveCodeRepository, batchSize int) error {
	if batchSize <= 0 {
		batchSize = 1000
	}

	if err := f.client.Del(ctx, redisFilterTempKey).Err(); err != nil {
		return err
	}
	if err := f.client.SAdd(ctx, redisFilterTempKey, redisFilterMarker).Err(); err != nil {
		return err
	}

	if err := source.ListActiveCodes(ctx, batchSize, func(codes []string) error {
		values := make([]any, 0, len(codes))
		for _, code := range codes {
			values = append(values, code)
		}
		if len(values) == 0 {
			return nil
		}
		return f.client.SAdd(ctx, redisFilterTempKey, values...).Err()
	}); err != nil {
		_ = f.client.Del(ctx, redisFilterTempKey).Err()
		return err
	}

	if err := f.client.Rename(ctx, redisFilterTempKey, redisFilterKey).Err(); err != nil {
		return err
	}
	return f.client.SRem(ctx, redisFilterKey, redisFilterMarker).Err()
}
