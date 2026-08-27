package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"golang.org/x/time/rate"
)

/*
RateLimiter behavior:
- Per-IP (or X-Forwarded-For resolved by Gin) limiting
- Default: 60 req/min, burst 20
- Redis-backed if redisURL provided (recommended for production)
*/

type RateLimiterOptions struct {
	RequestsPerMinute int
	Burst             int
	RedisURL          string
	Prefix            string // e.g. "rl"
	Window            time.Duration
	Message           string
	SkipPaths         []string
	SkipPathPrefixes  []string
}

func RateLimiter(opts RateLimiterOptions) gin.HandlerFunc {
	if opts.RequestsPerMinute <= 0 {
		opts.RequestsPerMinute = 60
	}
	if opts.Burst <= 0 {
		opts.Burst = 20
	}
	if opts.Prefix == "" {
		opts.Prefix = "rl"
	}
	if opts.Window <= 0 {
		opts.Window = time.Minute
	}
	if strings.TrimSpace(opts.Message) == "" {
		opts.Message = "Too many requests. Please wait a moment and try again."
	}

	// Redis-backed
	if strings.TrimSpace(opts.RedisURL) != "" {
		rdb := newRedisClient(opts.RedisURL)
		if rdb != nil {
			return redisRateLimiter(rdb, opts)
		}
		// fallthrough to memory if redis init failed
	}

	return memoryRateLimiter(opts)
}

func newRedisClient(redisURL string) *redis.Client {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil
	}
	rdb := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil
	}
	return rdb
}

/* =========================
   Redis Limiter
========================= */

func redisRateLimiter(rdb *redis.Client, opts RateLimiterOptions) gin.HandlerFunc {
	// Fixed-window limiting using INCR and a one-time EXPIRE.
	// This keeps the retry window stable instead of extending it on every retry.
	limit := int64(opts.RequestsPerMinute)
	window := opts.Window

	return func(c *gin.Context) {
		if shouldSkipRateLimit(c.Request.URL.Path, opts.SkipPaths, opts.SkipPathPrefixes) {
			c.Next()
			return
		}

		ip := c.ClientIP()
		key := redisKey(opts.Prefix, ip)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 200*time.Millisecond)
		defer cancel()

		count, err := rdb.Incr(ctx, key).Result()
		if err != nil {
			// If Redis has a hiccup, fail OPEN (do not take down API).
			c.Next()
			return
		}

		if count == 1 {
			if err := rdb.Expire(ctx, key, window).Err(); err != nil {
				// If Redis has a hiccup, fail OPEN (do not take down API).
				c.Next()
				return
			}
		}

		if count > limit {
			ttl, err := rdb.TTL(ctx, key).Result()
			if err != nil {
				ttl = window
			}
			writeRateLimitResponse(c, opts.Message, retryAfterForDuration(ttl))
			c.Abort()
			return
		}

		c.Next()
	}
}

func redisKey(prefix, ip string) string {
	// Avoid storing raw IPs (minor privacy hardening)
	h := sha256.Sum256([]byte(ip))
	return prefix + ":" + hex.EncodeToString(h[:])
}

/* =========================
   In-memory Limiter (fallback)
========================= */

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

var (
	memOnce     sync.Once
	memVisitors map[string]*visitor
	memMu       sync.Mutex
)

func memoryRateLimiter(opts RateLimiterOptions) gin.HandlerFunc {
	memOnce.Do(func() {
		memVisitors = make(map[string]*visitor)

		// Cleanup goroutine ONCE
		go func() {
			ticker := time.NewTicker(2 * time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				memMu.Lock()
				for ip, v := range memVisitors {
					if time.Since(v.lastSeen) > 5*time.Minute {
						delete(memVisitors, ip)
					}
				}
				memMu.Unlock()
			}
		}()
	})

	r := rate.Every(opts.Window / time.Duration(opts.RequestsPerMinute))

	return func(c *gin.Context) {
		if shouldSkipRateLimit(c.Request.URL.Path, opts.SkipPaths, opts.SkipPathPrefixes) {
			c.Next()
			return
		}

		ip := c.ClientIP()
		visitorKey := opts.Prefix + ":" + ip

		memMu.Lock()
		v, exists := memVisitors[visitorKey]
		if !exists {
			v = &visitor{
				limiter: rate.NewLimiter(r, opts.Burst),
			}
			memVisitors[visitorKey] = v
		}
		v.lastSeen = time.Now()
		lim := v.limiter
		memMu.Unlock()

		if !lim.Allow() {
			writeRateLimitResponse(c, opts.Message, retryAfterForWindow(opts.Window))
			c.Abort()
			return
		}

		c.Next()
	}
}

func shouldSkipRateLimit(path string, exactPaths, prefixes []string) bool {
	for _, exact := range exactPaths {
		if path == strings.TrimSpace(exact) {
			return true
		}
	}

	for _, prefix := range prefixes {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" {
			continue
		}
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func retryAfterForWindow(window time.Duration) int {
	return retryAfterForDuration(window)
}

func retryAfterForDuration(duration time.Duration) int {
	seconds := int(duration / time.Second)
	if seconds < 1 {
		return 1
	}
	return seconds
}

func writeRateLimitResponse(c *gin.Context, message string, retryAfterSeconds int) {
	c.Header("Retry-After", strconv.Itoa(retryAfterSeconds))
	c.JSON(http.StatusTooManyRequests, gin.H{
		"error":             "Too Many Requests",
		"message":           message,
		"statusCode":        http.StatusTooManyRequests,
		"retryAfterSeconds": retryAfterSeconds,
	})
}
