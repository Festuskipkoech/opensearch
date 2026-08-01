package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"opensearch/internal/types"
)

// ErrCacheMiss is returned when a key does not exist in the cache.
var ErrCacheMiss = errors.New("cache miss")

// Cache wraps the Redis client. One instance lives for the server lifetime.
// Safe for concurrent use — the Redis client manages its own connection pool.
type Cache struct {
	client *redis.Client
}

// New creates a Cache from a Redis URL.
func New(redisURL string, db int) (*Cache, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid redis URL: %w", err)
	}
	opts.DB = db
	return &Cache{client: redis.NewClient(opts)}, nil
}

// Ping verifies Redis is reachable. Called during startup health check.
func (c *Cache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// Get retrieves and deserialises a cached Response.
// Returns ErrCacheMiss when the key does not exist.
func (c *Cache) Get(ctx context.Context, key string) (types.Response, error) {
	val, err := c.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return types.Response{}, ErrCacheMiss
	}
	if err != nil {
		return types.Response{}, fmt.Errorf("cache get %q: %w", key, err)
	}
	var r types.Response
	if err := json.Unmarshal(val, &r); err != nil {
		return types.Response{}, fmt.Errorf("cache unmarshal %q: %w", key, err)
	}
	return r, nil
}

// Set serialises and stores a Response with a TTL.
// TTL must be greater than zero — zero TTL means no expiry which is never
// correct for search results.
func (c *Cache) Set(ctx context.Context, key string, value types.Response, ttl time.Duration) error {
	if ttl <= 0 {
		return fmt.Errorf("cache set %q: TTL must be greater than zero", key)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache marshal %q: %w", key, err)
	}
	if err := c.client.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("cache set %q: %w", key, err)
	}
	return nil
}

// Close closes the Redis connection. Called during graceful shutdown.
func (c *Cache) Close() error {
	return c.client.Close()
}