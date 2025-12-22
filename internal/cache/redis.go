package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

type RedisClient struct {
    client *redis.Client
    ctx    context.Context
}

// NewRedisClient creates a new Redis client
func NewRedisClient(redisURL string, poolSize int) (*RedisClient, error) {
    opts, err := redis.ParseURL(redisURL)
    if err != nil {
        return nil, fmt.Errorf("failed to parse redis URL: %w", err)
    }
    
    opts.PoolSize = poolSize
    opts.MinIdleConns = 5
    
    client := redis.NewClient(opts)
    ctx := context.Background()
    
    // Test connection
    if err := client.Ping(ctx).Err(); err != nil {
        return nil, fmt.Errorf("failed to connect to redis: %w", err)
    }
    
    return &RedisClient{
        client: client,
        ctx:    ctx,
    }, nil
}

// Basic operations
func (r *RedisClient) Set(key string, value interface{}, expiration time.Duration) error {
    return r.client.Set(r.ctx, key, value, expiration).Err()
}

func (r *RedisClient) Get(key string) (string, error) {
    return r.client.Get(r.ctx, key).Result()
}

func (r *RedisClient) Delete(key string) error {
    return r.client.Del(r.ctx, key).Err()
}

func (r *RedisClient) Exists(key string) bool {
    return r.client.Exists(r.ctx, key).Val() > 0
}

// Cache specific operations
func (r *RedisClient) SetJSON(key string, value interface{}, expiration time.Duration) error {
    jsonData, err := json.Marshal(value)
    if err != nil {
        return err
    }
    return r.Set(key, jsonData, expiration)
}

func (r *RedisClient) GetJSON(key string, dest interface{}) error {
    data, err := r.Get(key)
    if err != nil {
        return err
    }
    return json.Unmarshal([]byte(data), dest)
}

// Rate limiting
func (r *RedisClient) RateLimit(key string, limit int, window time.Duration) (bool, error) {
    now := time.Now().UnixNano()
    windowMicro := window.Microseconds()
    
    pipe := r.client.Pipeline()
    
    // Remove old requests
    pipe.ZRemRangeByScore(r.ctx, key, "0", fmt.Sprintf("%d", now-windowMicro))
    
    // Count current requests
    countCmd := pipe.ZCard(r.ctx, key)
    
    // Add current request
    pipe.ZAdd(r.ctx, key, &redis.Z{
        Score:  float64(now),
        Member: now,
    })
    
    // Set expiry
    pipe.Expire(r.ctx, key, window)
    
    _, err := pipe.Exec(r.ctx)
    if err != nil {
        return false, err
    }
    
    count := countCmd.Val()
    return count <= int64(limit), nil
}

// Close connection
func (r *RedisClient) Close() error {
    return r.client.Close()
}