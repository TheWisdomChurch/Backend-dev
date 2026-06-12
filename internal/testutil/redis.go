package testutil

import (
	"testing"

	"github.com/alicebob/miniredis/v2"

	"wisdomHouse-backend/internal/cache"
)

// NewRedis starts an in-process miniredis server and returns a *cache.RedisClient
// connected to it (the type used throughout the application) plus the raw
// *miniredis.Miniredis for time-travel helpers like FastForward.
// Both are closed/stopped in t.Cleanup.
func NewRedis(t *testing.T) (*cache.RedisClient, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("testutil.NewRedis: start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	client, err := cache.NewRedisClient(cache.Config{
		URL: "redis://" + mr.Addr(),
	})
	if err != nil {
		t.Fatalf("testutil.NewRedis: NewRedisClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	return client, mr
}
