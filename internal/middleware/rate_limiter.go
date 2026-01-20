package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

var (
	visitors = make(map[string]*visitor)
	mu       sync.Mutex
)

// RateLimiter: 60 req/min with burst 20 per IP
func RateLimiter() gin.HandlerFunc {
	// Cleanup goroutine
	go func() {
		for {
			time.Sleep(2 * time.Minute)
			mu.Lock()
			for ip, v := range visitors {
				if time.Since(v.lastSeen) > 5*time.Minute {
					delete(visitors, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return func(c *gin.Context) {
		ip := c.ClientIP()

		mu.Lock()
		v, exists := visitors[ip]
		if !exists {
			v = &visitor{
				limiter: rate.NewLimiter(rate.Every(time.Minute/60), 20),
			}
			visitors[ip] = v
		}
		v.lastSeen = time.Now()
		lim := v.limiter
		mu.Unlock()

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
