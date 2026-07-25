package cache

import (
	"context"
	"errors"
	"fmt"
	"github.com/redis/go-redis/v9"
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

// Ping verifies Redis is reachable. Called during startup before
// the server accepts traffic.
func (c *Cache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// Close closes the Redis connection. Called during graceful shutdown.
func (c *Cache) Close() error {
	return c.client.Close()
}