package middleware

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"net/http"
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
	// Token-bucket approximation using fixed window with INCR + EXPIRE.
	// This is extremely fast and good enough for API abuse protection.
	limit := int64(opts.RequestsPerMinute)
	window := time.Minute

	return func(c *gin.Context) {
		ip := c.ClientIP()
		key := redisKey(opts.Prefix, ip)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 200*time.Millisecond)
		defer cancel()

		pipe := rdb.TxPipeline()
		incr := pipe.Incr(ctx, key)
		pipe.Expire(ctx, key, window)
		_, err := pipe.Exec(ctx)

		if err != nil {
			// If Redis has a hiccup, fail OPEN (do not take down API).
			c.Next()
			return
		}

		if incr.Val() > limit {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":      "Too Many Requests",
				"message":    "Rate limit exceeded",
				"statusCode": http.StatusTooManyRequests,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

func redisKey(prefix, ip string) string {
	// Avoid storing raw IPs (minor privacy hardening)
	h := sha1.Sum([]byte(ip))
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

	r := rate.Every(time.Minute / time.Duration(opts.RequestsPerMinute))

	return func(c *gin.Context) {
		ip := c.ClientIP()

		memMu.Lock()
		v, exists := memVisitors[ip]
		if !exists {
			v = &visitor{
				limiter: rate.NewLimiter(r, opts.Burst),
			}
			memVisitors[ip] = v
		}
		v.lastSeen = time.Now()
		lim := v.limiter
		memMu.Unlock()

		if !lim.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":      "Too Many Requests",
				"message":    "Rate limit exceeded",
				"statusCode": http.StatusTooManyRequests,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
