package middleware

import "github.com/gin-gonic/gin"

func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		path := c.Request.URL.Path
		if path == "/swagger/" || path == "/swagger/index.html" || (len(path) >= 9 && path[:9] == "/swagger/") {
			c.Header("Content-Security-Policy",
				"default-src 'self' 'unsafe-inline'; "+
					"script-src 'self' 'unsafe-inline' 'unsafe-eval'; "+
					"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; "+
					"font-src 'self' https://fonts.gstatic.com; "+
					"img-src 'self' data: https:;")
		} else {
			c.Header("Content-Security-Policy",
				"default-src 'none'; "+
					"frame-ancestors 'none'; "+
					"base-uri 'self';")
		}

		c.Next()
	}
}
