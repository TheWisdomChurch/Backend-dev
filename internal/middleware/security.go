// internal/middleware/security.go
package middleware

import "github.com/gin-gonic/gin"

// SecurityHeaders adds security headers to responses
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Security headers
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		
		// CSP Header (adjust based on your needs)
		if c.Request.URL.Path == "/swagger/" || c.Request.URL.Path == "/swagger/index.html" {
			// Relaxed CSP for Swagger UI
			c.Header("Content-Security-Policy", 
				"default-src 'self' 'unsafe-inline'; "+
				"script-src 'self' 'unsafe-inline' 'unsafe-eval'; "+
				"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; "+
				"font-src 'self' https://fonts.gstatic.com; "+
				"img-src 'self' data: https:;")
		} else {
			// Stricter CSP for regular endpoints
			c.Header("Content-Security-Policy", 
				"default-src 'self'; "+
				"script-src 'self'; "+
				"style-src 'self'; "+
				"img-src 'self' data:;")
		}

		c.Next()
	}
}