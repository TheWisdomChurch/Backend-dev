package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/repository"
)

const sessionFreshnessLeeway = 30 * time.Second

// SessionFreshnessMiddleware enforces "latest login wins".
// Any token issued before the user's current last_login_at is treated as revoked.
func SessionFreshnessMiddleware(userRepo repository.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		if userRepo == nil {
			c.Next()
			return
		}

		rawClaims, exists := c.Get("auth_claims")
		if !exists {
			c.Next()
			return
		}

		claims, ok := rawClaims.(*AccessClaims)
		if !ok || claims == nil || claims.IssuedAt == nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":      "Unauthorized",
				"message":    "Invalid session",
				"statusCode": http.StatusUnauthorized,
			})
			c.Abort()
			return
		}

		user, err := userRepo.FindByID(claims.UserID)
		if err != nil || user == nil || !user.IsActive {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":      "Unauthorized",
				"message":    "Invalid session",
				"statusCode": http.StatusUnauthorized,
			})
			c.Abort()
			return
		}

		if user.LastLoginAt != nil {
			cutoff := user.LastLoginAt.UTC().Add(-sessionFreshnessLeeway)
			if claims.IssuedAt.Time.UTC().Before(cutoff) {
				c.JSON(http.StatusUnauthorized, gin.H{
					"error":      "Unauthorized",
					"message":    "Session expired due to a newer login",
					"statusCode": http.StatusUnauthorized,
				})
				c.Abort()
				return
			}
		}

		c.Next()
	}
}
