package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func RequestBodyLimit(maxBytes int64) gin.HandlerFunc {
	if maxBytes <= 0 {
		maxBytes = 2 << 20
	}

	return func(c *gin.Context) {
		if c.Request.ContentLength > maxBytes {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
				"error":      "Payload Too Large",
				"message":    "Request body exceeds the configured limit",
				"statusCode": http.StatusRequestEntityTooLarge,
			})
			return
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}
