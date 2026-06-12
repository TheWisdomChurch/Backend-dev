//go:build integration

package tests

import (
	"context"
	"testing"
	"time"

	"wisdomHouse-backend/internal/authutil"
	"wisdomHouse-backend/internal/cache"
	"wisdomHouse-backend/internal/testutil"
)

// TestTokenBlocklist_BlockAndCheck verifies that a JTI blocked in Redis is
// immediately seen as blocked and that the block expires after the TTL.
func TestTokenBlocklist_BlockAndCheck(t *testing.T) {
	redisClient, mr := testutil.NewRedis(t)
	blocklist := authutil.NewTokenBlocklist(redisClient)
	ctx := context.Background()

	jti := "test-jti-001"

	// Before blocking — must not be blocked.
	if blocklist.IsBlocked(ctx, jti) {
		t.Fatal("jti must not be blocked before Block() is called")
	}

	// Block with a 2-second TTL.
	if err := blocklist.Block(ctx, jti, 2*time.Second); err != nil {
		t.Fatalf("Block: %v", err)
	}

	// Immediately after blocking — must be blocked.
	if !blocklist.IsBlocked(ctx, jti) {
		t.Fatal("jti must be blocked immediately after Block()")
	}

	// Advance miniredis clock past TTL and check expiry.
	mr.FastForward(3 * time.Second)

	if blocklist.IsBlocked(ctx, jti) {
		t.Fatal("jti should no longer be blocked after TTL expiry")
	}
}

// TestTokenBlocklist_MultipleJTIs verifies independent JTIs do not interfere.
func TestTokenBlocklist_MultipleJTIs(t *testing.T) {
	redisClient, _ := testutil.NewRedis(t)
	blocklist := authutil.NewTokenBlocklist(redisClient)
	ctx := context.Background()

	a, b := "jti-alpha", "jti-beta"

	if err := blocklist.Block(ctx, a, time.Minute); err != nil {
		t.Fatalf("Block a: %v", err)
	}

	if !blocklist.IsBlocked(ctx, a) {
		t.Error("jti-alpha must be blocked")
	}
	if blocklist.IsBlocked(ctx, b) {
		t.Error("jti-beta must NOT be blocked")
	}
}

// TestTokenBlocklist_EmptyJTI verifies empty JTI is never blocked (guard clause).
func TestTokenBlocklist_EmptyJTI(t *testing.T) {
	redisClient, _ := testutil.NewRedis(t)
	blocklist := authutil.NewTokenBlocklist(redisClient)
	ctx := context.Background()

	// Block("", ...) must be a no-op — no error.
	if err := blocklist.Block(ctx, "", time.Minute); err != nil {
		t.Fatalf("Block(\"\", ...): expected no error, got %v", err)
	}
	if blocklist.IsBlocked(ctx, "") {
		t.Error("empty JTI must never be blocked")
	}
}

// TestUserCache_RoundTrip verifies the Redis user session cache stores and retrieves state.
func TestUserCache_RoundTrip(t *testing.T) {
	redisClient, _ := testutil.NewRedis(t)
	userCache := cache.NewUserCache(redisClient, 0) // 0 → DefaultUserCacheTTL
	ctx := context.Background()

	userID := "test-user-uuid-001"
	state := &cache.CachedUserState{IsActive: true, AdminApproved: true}

	if err := userCache.Set(ctx, userID, state); err != nil {
		t.Fatalf("UserCache.Set: %v", err)
	}

	got, err := userCache.Get(ctx, userID)
	if err != nil {
		t.Fatalf("UserCache.Get: %v", err)
	}
	if got == nil {
		t.Fatal("UserCache.Get returned nil after Set")
	}
	if got.IsActive != state.IsActive || got.AdminApproved != state.AdminApproved {
		t.Errorf("UserCache round-trip: want %+v, got %+v", *state, *got)
	}
}

// TestUserCache_Invalidate verifies Invalidate removes the cached entry.
func TestUserCache_Invalidate(t *testing.T) {
	redisClient, _ := testutil.NewRedis(t)
	userCache := cache.NewUserCache(redisClient, 0)
	ctx := context.Background()

	userID := "test-user-uuid-002"

	if err := userCache.Set(ctx, userID, &cache.CachedUserState{IsActive: true}); err != nil {
		t.Fatalf("UserCache.Set: %v", err)
	}
	if err := userCache.Invalidate(ctx, userID); err != nil {
		t.Fatalf("UserCache.Invalidate: %v", err)
	}

	got, err := userCache.Get(ctx, userID)
	if err != nil {
		t.Fatalf("UserCache.Get after Invalidate: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil after Invalidate, got %+v", got)
	}
}
