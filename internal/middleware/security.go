package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

func SecurityHeaders(isProduction bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "0")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Cross-Origin-Opener-Policy", "same-origin")
		c.Header("Cross-Origin-Resource-Policy", "same-origin")
		c.Header("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		if isProduction {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		path := c.Request.URL.Path
		if isPresentationPage(path) {
			c.Header("Content-Security-Policy",
				"default-src 'self'; "+
					"frame-ancestors 'none'; "+
					"base-uri 'self'; "+
					"form-action 'self'; "+
					"connect-src 'self'; "+
					"script-src 'self' 'unsafe-inline'; "+
					"style-src 'self' 'unsafe-inline'; "+
					"img-src 'self' data: https:; "+
					"font-src 'self' data: https:;")
		} else {
			c.Header("Content-Security-Policy",
				"default-src 'none'; "+
					"frame-ancestors 'none'; "+
					"base-uri 'self';")
		}

		c.Next()
	}
}

func isPresentationPage(path string) bool {
	return strings.HasPrefix(path, "/forms/") ||
		strings.HasPrefix(path, "/form/") ||
		strings.HasPrefix(path, "/reports/forms/") ||
		strings.HasPrefix(path, "/api/v1/auth/oauth/google/")
}
