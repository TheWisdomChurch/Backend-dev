package authutil

import (
	"context"
	"time"

	"wisdomHouse-backend/internal/cache"
)

const jtiBlockKeyPrefix = "jti:block:"

// TokenBlocklist tracks revoked JWT IDs in Redis so that logout immediately
// invalidates the token regardless of its remaining lifetime.
//
// If Redis is unavailable, Block returns an error but IsBlocked returns false
// (fail open) — a conservative choice: a revoked token may still work briefly,
// but legitimate users are never locked out by a Redis outage.
type TokenBlocklist struct {
	redis *cache.RedisClient
}

// NewTokenBlocklist creates a blocklist backed by the given Redis client.
func NewTokenBlocklist(r *cache.RedisClient) *TokenBlocklist {
	return &TokenBlocklist{redis: r}
}

// Block adds a JTI to the blocklist. ttl should be the token's remaining valid
// duration so the key expires automatically when the token would have expired anyway.
func (b *TokenBlocklist) Block(ctx context.Context, jti string, ttl time.Duration) error {
	if jti == "" || ttl <= 0 {
		return nil
	}
	return b.redis.Set(ctx, jtiBlockKeyPrefix+jti, "1", ttl)
}

// IsBlocked returns true if the JTI has been explicitly revoked.
// Returns false (and logs nothing) on Redis errors — fail open.
func (b *TokenBlocklist) IsBlocked(ctx context.Context, jti string) bool {
	if jti == "" {
		return false
	}
	blocked, err := b.redis.Exists(ctx, jtiBlockKeyPrefix+jti)
	if err != nil {
		return false
	}
	return blocked
}
