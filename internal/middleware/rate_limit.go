package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimitConfig defines limits per endpoint
var RateLimitConfig = map[string]RateLimit{
	// Auth endpoints - generous limits for good UX
	"/api/v1/auth/me": {
		RequestsPerSecond: 10,  // 10 req/sec per user
		BurstSize:        50,   // Allow bursts of 50
		Window:           1 * time.Second,
	},
	"/api/v1/auth/login/verify-otp": {
		RequestsPerSecond: 5,  // 5 attempts per second
		BurstSize:        10,  // Max 10 in quick succession
		Window:           1 * time.Second,
	},
	"/api/v1/auth/login": {
		RequestsPerSecond: 3, // 3 logins per second
		BurstSize:        5,  // Burst up to 5
		Window:           1 * time.Second,
	},
}

type RateLimit struct {
	RequestsPerSecond float64
	BurstSize        int
	Window           time.Duration
}

// RateLimitMiddleware returns per-user rate limiting middleware
func RateLimitMiddleware(endpoint string) gin.HandlerFunc {
	limiters := make(map[string]*rate.Limiter)

	config, exists := RateLimitConfig[endpoint]
	if !exists {
		// Default: 100 req/sec if not configured
		config = RateLimit{
			RequestsPerSecond: 100,
			BurstSize:        200,
			Window:           1 * time.Second,
		}
	}

	return func(c *gin.Context) {
		// Use user ID or IP for rate limiting
		identifier := getUserIdentifier(c)

		// Get or create limiter for this user
		if _, exists := limiters[identifier]; !exists {
			limiters[identifier] = rate.NewLimiter(
				rate.Limit(config.RequestsPerSecond),
				config.BurstSize,
			)
		}

		if !limiters[identifier].Allow() {
			// Return retry-after header
			c.Header("Retry-After", "1")
			c.JSON(http.StatusTooManyRequests, gin.H{
				"status":  "error",
				"message": "Rate limit exceeded. Please wait a moment.",
				"meta": gin.H{
					"retry_after": 1,
					"help":        "You're making requests too quickly. Please wait 1 second.",
				},
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

func getUserIdentifier(c *gin.Context) string {
	// Prefer user ID from JWT
	if userID, exists := c.Get("user_id"); exists {
		return fmt.Sprintf("user:%v", userID)
	}

	// Fall back to email from JWT
	if email, exists := c.Get("email"); exists {
		return fmt.Sprintf("email:%s", email)
	}

	// Fall back to IP address
	ip := c.ClientIP()
	if ip != "" {
		return fmt.Sprintf("ip:%s", ip)
	}

	return "unknown"
}

// ApplyRateLimitToRoute registers rate limiting for a specific route
func ApplyRateLimitToRoute(g *gin.RouterGroup, method, path string) {
	endpoint := fmt.Sprintf("/api/v1%s", path)
	middleware := RateLimitMiddleware(endpoint)

	// Register with rate limiting
	switch strings.ToUpper(method) {
	case "GET":
		g.GET(path, middleware)
	case "POST":
		g.POST(path, middleware)
	case "PUT":
		g.PUT(path, middleware)
	case "DELETE":
		g.DELETE(path, middleware)
	}
}
