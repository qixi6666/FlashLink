package filter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jd/flashlink/internal/domain/link"
	"github.com/redis/go-redis/v9"
)

const (
	defaultBloomFilterKey       = "flashlink:filter:codes"
	defaultBloomFilterCapacity  = uint64(10_000_000)
	defaultBloomFilterErrorRate = 0.01
	finishBloomRebuildScript    = `
if redis.call("GET", KEYS[3]) == ARGV[1] then
	redis.call("RENAME", KEYS[1], KEYS[2])
	redis.call("DEL", KEYS[3])
	return 1
end
return 0
`
)

type RedisBloomOptions struct {
	Key       string
	Capacity  uint64
	ErrorRate float64
}

type RedisBloom struct {
	client    *redis.Client
	key       string
	tempKey   string
	markerKey string
	capacity  int64
	errorRate float64
}

func NewRedisBloom(client *redis.Client, options RedisBloomOptions) *RedisBloom {
	key := options.Key
	if key == "" {
		key = defaultBloomFilterKey
	}

	capacity := options.Capacity
	if capacity == 0 {
		capacity = defaultBloomFilterCapacity
	}

	errorRate := options.ErrorRate
	if errorRate <= 0 || errorRate >= 1 {
		errorRate = defaultBloomFilterErrorRate
	}

	return &RedisBloom{
		client:    client,
		key:       key,
		tempKey:   key + ":rebuild",
		markerKey: key + ":rebuilding",
		capacity:  int64(capacity),
		errorRate: errorRate,
	}
}

func (f *RedisBloom) Add(ctx context.Context, code string) error {
	keys, err := f.addKeys(ctx)
	if err != nil {
		return err
	}
	return f.addToKeys(ctx, keys, code)
}

func (f *RedisBloom) MightContain(ctx context.Context, code string) (bool, error) {
	return f.client.BFExists(ctx, f.key, code).Result()
}

func (f *RedisBloom) Rebuild(ctx context.Context, source link.ActiveCodeRepository, batchSize int) error {
	if batchSize <= 0 {
		batchSize = 1000
	}

	tempKey := f.rebuildTempKey()
	if err := f.client.Set(ctx, f.markerKey, tempKey, 0).Err(); err != nil {
		return err
	}
	if err := f.reserve(ctx, tempKey); err != nil {
		_ = f.client.Del(ctx, f.markerKey).Err()
		return err
	}

	if err := source.ListActiveCodes(ctx, batchSize, func(codes []string) error {
		return f.addBatchToKey(ctx, tempKey, codes)
	}); err != nil {
		_ = f.client.Del(ctx, tempKey).Err()
		_ = f.client.Del(ctx, f.markerKey).Err()
		return err
	}

	return f.finishRebuild(ctx, tempKey)
}

func (f *RedisBloom) rebuildTempKey() string {
	return fmt.Sprintf("%s:%d", f.tempKey, time.Now().UnixNano())
}

func (f *RedisBloom) finishRebuild(ctx context.Context, tempKey string) error {
	return f.client.Eval(ctx, finishBloomRebuildScript, []string{tempKey, f.key, f.markerKey}, tempKey).Err()
}

func (f *RedisBloom) addKeys(ctx context.Context) ([]string, error) {
	tempKey, err := f.client.Get(ctx, f.markerKey).Result()
	if errors.Is(err, redis.Nil) {
		return []string{f.key}, nil
	}
	if err != nil {
		return nil, err
	}
	if tempKey == "" {
		return []string{f.key}, nil
	}
	return []string{f.key, tempKey}, nil
}

func (f *RedisBloom) addToKeys(ctx context.Context, keys []string, code string) error {
	for _, key := range dedupeStrings(keys) {
		if err := f.client.BFAdd(ctx, key, code).Err(); err != nil {
			return err
		}
	}
	return nil
}

func (f *RedisBloom) addBatchToKey(ctx context.Context, key string, codes []string) error {
	if len(codes) == 0 {
		return nil
	}

	return f.client.BFMAdd(ctx, key, stringsToInterfaces(codes)...).Err()
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	deduped := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		deduped = append(deduped, value)
	}
	return deduped
}

func (f *RedisBloom) reserve(ctx context.Context, key string) error {
	err := f.client.BFReserve(ctx, key, f.errorRate, f.capacity).Err()
	if err == nil || redis.HasErrorPrefix(err, "ERR item exists") {
		return nil
	}
	return err
}

func stringsToInterfaces(values []string) []interface{} {
	items := make([]interface{}, 0, len(values))
	for _, value := range values {
		items = append(items, value)
	}
	return items
}
