package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

var (
	ErrCacheMiss = redis.Nil
)

type Config struct {
	URL      string
	PoolSize int

	// Timeouts
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	PoolTimeout  time.Duration

	// Per-operation context timeout (safety net)
	OpTimeout time.Duration
}

type RedisClient struct {
	client    *redis.Client
	opTimeout time.Duration
}

// NewRedisClient creates a production-ready Redis client
func NewRedisClient(cfg Config) (*RedisClient, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("redis url is required")
	}
	if cfg.PoolSize <= 0 {
		cfg.PoolSize = 20
	}
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = 5 * time.Second
	}
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 3 * time.Second
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 3 * time.Second
	}
	if cfg.PoolTimeout == 0 {
		cfg.PoolTimeout = 4 * time.Second
	}
	if cfg.OpTimeout == 0 {
		cfg.OpTimeout = 2 * time.Second
	}

	opts, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis URL: %w", err)
	}

	opts.PoolSize = cfg.PoolSize
	opts.MinIdleConns = max(5, cfg.PoolSize/4)

	opts.DialTimeout = cfg.DialTimeout
	opts.ReadTimeout = cfg.ReadTimeout
	opts.WriteTimeout = cfg.WriteTimeout
	opts.PoolTimeout = cfg.PoolTimeout

	// Health + reconnection friendliness
	opts.MaxRetries = 3
	opts.MinRetryBackoff = 200 * time.Millisecond
	opts.MaxRetryBackoff = 2 * time.Second

	// Keep connections fresh
	opts.IdleTimeout = 5 * time.Minute
	opts.IdleCheckFrequency = 1 * time.Minute

	client := redis.NewClient(opts)

	// Startup ping (fail fast)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &RedisClient{client: client, opTimeout: cfg.OpTimeout}, nil
}

// WithTimeout wraps a context with the client's default op timeout.
func (r *RedisClient) WithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	// If caller already has a deadline, respect it
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, r.opTimeout)
}

// Set stores a value with expiration
func (r *RedisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	cctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.client.Set(cctx, key, value, expiration).Err()
}

// Get returns a value or ErrCacheMiss (redis.Nil)
func (r *RedisClient) Get(ctx context.Context, key string) (string, error) {
	cctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.client.Get(cctx, key).Result()
}

func (r *RedisClient) Delete(ctx context.Context, key string) error {
	cctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.client.Del(cctx, key).Err()
}

func (r *RedisClient) Exists(ctx context.Context, key string) (bool, error) {
	cctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	n, err := r.client.Exists(cctx, key).Result()
	return n > 0, err
}

// SetJSON stores a struct/map as JSON
func (r *RedisClient) SetJSON(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return r.Set(ctx, key, b, expiration)
}

// GetJSON loads JSON into dest
func (r *RedisClient) GetJSON(ctx context.Context, key string, dest interface{}) error {
	s, err := r.Get(ctx, key)
	if err != nil {
		return err
	}

	// Handle when stored as []byte by SetJSON (string contains bytes)
	return json.Unmarshal([]byte(s), dest)
}

// RateLimit implements a production-safe sliding window limiter using Lua.
// Returns (allowed, error).
//
// It stores timestamps in a ZSET and keeps window sized by pruning old entries.
// This is atomic and avoids race conditions from pipelines.
func (r *RedisClient) RateLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	if limit <= 0 {
		return false, errors.New("limit must be > 0")
	}
	if window <= 0 {
		return false, errors.New("window must be > 0")
	}

	cctx, cancel := r.WithTimeout(ctx)
	defer cancel()

	// Lua script:
	// - remove old entries
	// - add now
	// - count
	// - set expiry
	// - return 1 if allowed else 0
	script := redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])

redis.call("ZREMRANGEBYSCORE", key, 0, now - window)
redis.call("ZADD", key, now, now)
local count = redis.call("ZCARD", key)
redis.call("PEXPIRE", key, window)

if count <= limit then
  return 1
else
  return 0
end
`)

	nowMs := time.Now().UnixMilli()
	windowMs := window.Milliseconds()

	res, err := script.Run(cctx, r.client, []string{key}, nowMs, windowMs, limit).Int()
	if err != nil {
		return false, err
	}
	return res == 1, nil
}

func (r *RedisClient) Close() error {
	return r.client.Close()
}

// Ping checks Redis connectivity. Used by the readiness endpoint.
func (r *RedisClient) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
