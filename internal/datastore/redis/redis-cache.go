package redis_cache

import (
	"context"
	"time"

	redis "github.com/redis/go-redis/v9"
)

const ExploreTTL = 30 * time.Second // TTL can probably be 30 seconds without impacting user experience, though this value should be the same across endpoints.

// In the real world we could have some kind of detailed caching policy, but for the sake of simplicity we will go with
// a single ttl value.
type RedisCache struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisCache(client *redis.Client, ttl time.Duration) *RedisCache {
	return &RedisCache{client: client, ttl: ttl}
}

func (r *RedisCache) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil // cache miss
	}
	if err != nil {
		return nil, err
	}
	return []byte(val), nil
}

func (r *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

func (r *RedisCache) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}
