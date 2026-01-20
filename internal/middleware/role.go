package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func RoleMiddleware(requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roleVal, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{
				"error":      "Forbidden",
				"message":    "User role not found",
				"statusCode": http.StatusForbidden,
			})
			c.Abort()
			return
		}

		userRole, ok := roleVal.(string)
		if !ok || userRole == "" {
			c.JSON(http.StatusForbidden, gin.H{
				"error":      "Forbidden",
				"message":    "Invalid role format",
				"statusCode": http.StatusForbidden,
			})
			c.Abort()
			return
		}

		if userRole != requiredRole && userRole != "super_admin" {
			c.JSON(http.StatusForbidden, gin.H{
				"error":         "Forbidden",
				"message":       "Insufficient permissions",
				"required_role": requiredRole,
				"user_role":     userRole,
				"statusCode":    http.StatusForbidden,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
