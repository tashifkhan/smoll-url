package server

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type redirectCache struct {
	client  *redis.Client
	prefix  string
	timeout time.Duration
}

func newRedirectCache(redisURL, keyPrefix string, timeout time.Duration) *redirectCache {
	redisURL = strings.TrimSpace(redisURL)
	if redisURL == "" {
		return nil
	}
	if timeout <= 0 {
		timeout = 200 * time.Millisecond
	}
	if keyPrefix == "" {
		keyPrefix = "smoll-url:redirect:"
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Printf("redis cache disabled (invalid redis_url): %v", err)
		return nil
	}

	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		log.Printf("redis cache disabled (cannot connect): %v", err)
		_ = client.Close()
		return nil
	}

	return &redirectCache{
		client:  client,
		prefix:  keyPrefix,
		timeout: timeout,
	}
}

func (c *redirectCache) get(shortlink string) (string, bool) {
	if c == nil {
		return "", false
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	val, err := c.client.Get(ctx, c.key(shortlink)).Result()
	if err == redis.Nil {
		return "", false
	}
	if err != nil {
		return "", false
	}

	return val, true
}

func (c *redirectCache) set(shortlink, longURL string, expiryTime int64) {
	if c == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	expiresIn := time.Duration(0)
	if expiryTime > 0 {
		expiresIn = time.Until(time.Unix(expiryTime, 0).UTC())
		if expiresIn <= 0 {
			_ = c.client.Del(ctx, c.key(shortlink)).Err()
			return
		}
	}

	if err := c.client.Set(ctx, c.key(shortlink), longURL, expiresIn).Err(); err != nil {
		return
	}
}

func (c *redirectCache) delete(shortlink string) {
	if c == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	_ = c.client.Del(ctx, c.key(shortlink)).Err()
}

func (c *redirectCache) close() error {
	if c == nil {
		return nil
	}
	return c.client.Close()
}

func (c *redirectCache) key(shortlink string) string {
	return c.prefix + shortlink
}
