// internal/middleware/role.go
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RoleMiddleware checks if user has required role
func RoleMiddleware(requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "Forbidden",
				"message": "User role not found",
			})
			c.Abort()
			return
		}

		if userRole != requiredRole && userRole != "super_admin" {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "Forbidden",
				"message": "Insufficient permissions",
				"required_role": requiredRole,
				"user_role": userRole,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}