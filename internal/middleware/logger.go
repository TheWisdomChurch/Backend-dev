package middleware

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

func Logger(level string) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		if raw != "" {
			path = path + "?" + raw
		}

		log.Printf("level=%s method=%s path=%s status=%d latency=%s ip=%s",
			level,
			c.Request.Method,
			path,
			c.Writer.Status(),
			time.Since(start),
			c.ClientIP(),
		)

		if len(c.Errors) > 0 {
			for _, e := range c.Errors.Errors() {
				log.Printf("level=error path=%s err=%q", path, e)
			}
		}
	}
}
