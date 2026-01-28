package middleware

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

func Logger(logLevel string) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		if raw != "" {
			path = path + "?" + raw
		}

		status := c.Writer.Status()
		latency := time.Since(start)
		method := c.Request.Method
		ip := c.ClientIP()

		// Minimal, production-friendly line
		// Example: method=GET path=/api/v1/auth/me status=200 latency=12ms ip=1.2.3.4
		log.Printf("level=%s method=%s path=%s status=%d latency=%s ip=%s",
			logLevel, method, path, status, latency, ip,
		)

		// Log gin errors if present
		if len(c.Errors) > 0 {
			for _, e := range c.Errors.Errors() {
				log.Printf("level=error path=%s err=%q", path, e)
			}
		}
	}
}
