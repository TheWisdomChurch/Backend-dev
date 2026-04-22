package middleware

import (
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func AuditLogger(scope string) gin.HandlerFunc {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = "api"
	}

	return func(c *gin.Context) {
		if !isMutatingMethod(c.Request.Method) {
			c.Next()
			return
		}

		start := time.Now()
		c.Next()

		requestID, _ := c.Get("request_id")
		userID, _ := c.Get("user_id")
		role, _ := c.Get("role")

		log.Printf(
			"level=audit scope=%s request_id=%v method=%s path=%s status=%d latency=%s user_id=%v role=%v ip=%s ua=%q",
			scope,
			requestID,
			c.Request.Method,
			c.Request.URL.Path,
			c.Writer.Status(),
			time.Since(start).Round(time.Millisecond),
			userID,
			role,
			c.ClientIP(),
			c.Request.UserAgent(),
		)
	}
}

func isMutatingMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	default:
		return false
	}
}
