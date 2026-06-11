package cache

import (
	"context"
	"errors"
	"time"
)

const (
	userSessionKeyPrefix = "user:session:"
	DefaultUserCacheTTL  = 60 * time.Second
)

// CachedUserState holds the minimal user fields needed by session middleware.
// TTL of 60s means deactivation/revocation propagates within 1 minute.
type CachedUserState struct {
	IsActive      bool       `json:"is_active"`
	AdminApproved bool       `json:"admin_approved"`
	LastLoginAt   *time.Time `json:"last_login_at,omitempty"`
}

// UserCache caches session-critical user state in Redis to avoid a DB hit on
// every authenticated request (session_freshness middleware).
type UserCache struct {
	redis *RedisClient
	ttl   time.Duration
}

// NewUserCache creates a UserCache with the given TTL (use DefaultUserCacheTTL if unsure).
func NewUserCache(r *RedisClient, ttl time.Duration) *UserCache {
	if ttl <= 0 {
		ttl = DefaultUserCacheTTL
	}
	return &UserCache{redis: r, ttl: ttl}
}

func userSessionKey(userID string) string {
	return userSessionKeyPrefix + userID
}

// Get retrieves cached user state. Returns (nil, nil) on cache miss.
func (c *UserCache) Get(ctx context.Context, userID string) (*CachedUserState, error) {
	var state CachedUserState
	err := c.redis.GetJSON(ctx, userSessionKey(userID), &state)
	if err != nil {
		if errors.Is(err, ErrCacheMiss) {
			return nil, nil
		}
		return nil, err
	}
	return &state, nil
}

// Set stores user state in Redis with the configured TTL.
func (c *UserCache) Set(ctx context.Context, userID string, state *CachedUserState) error {
	return c.redis.SetJSON(ctx, userSessionKey(userID), state, c.ttl)
}

// Invalidate removes a user's cached state. Call on logout, password change,
// account deactivation, and admin approval changes.
func (c *UserCache) Invalidate(ctx context.Context, userID string) error {
	return c.redis.Delete(ctx, userSessionKey(userID))
}
