package middleware

import "github.com/gin-gonic/gin"

// NoStore prevents browsers and intermediaries from persisting sensitive auth payloads.
func NoStore() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate, private")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.Next()
	}
}
