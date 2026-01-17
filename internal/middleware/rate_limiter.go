// internal/middleware/rate_limiter.go
package middleware

import "github.com/gin-gonic/gin"

// RateLimiter is a placeholder that does nothing
func RateLimiter() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Rate limiting logic to be implemented later
		c.Next()
	}
}