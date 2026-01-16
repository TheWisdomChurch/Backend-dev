// internal/middleware/requestid.go
package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestID adds a unique request ID to each request
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.Request.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}
		
		c.Set("X-Request-ID", requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}