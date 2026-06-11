package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/cache"
	"wisdomHouse-backend/internal/repository"
)

const sessionFreshnessLeeway = 30 * time.Second

// SessionFreshnessMiddleware enforces "latest login wins".
// Any token issued before the user's current last_login_at is treated as revoked.
//
// Uses a Redis cache (TTL 60s) to avoid a DB hit on every authenticated request.
// Falls back to a direct DB query on cache miss or when no cache is provided.
func SessionFreshnessMiddleware(userRepo repository.UserRepository, userCache *cache.UserCache) gin.HandlerFunc {
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

		ctx := c.Request.Context()
		isActive, adminApproved, lastLoginAt := resolveUserState(ctx, claims.UserID, userRepo, userCache)

		if !isActive || !adminApproved {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":      "Unauthorized",
				"message":    "Invalid session",
				"statusCode": http.StatusUnauthorized,
			})
			c.Abort()
			return
		}

		if lastLoginAt != nil {
			cutoff := lastLoginAt.UTC().Add(-sessionFreshnessLeeway)
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

// resolveUserState fetches session-critical fields from cache first, then DB on miss.
// On a DB hit it repopulates the cache for subsequent requests.
func resolveUserState(
	ctx context.Context,
	userID string,
	userRepo repository.UserRepository,
	userCache *cache.UserCache,
) (isActive, adminApproved bool, lastLoginAt *time.Time) {
	if userCache != nil {
		if state, err := userCache.Get(ctx, userID); err == nil && state != nil {
			return state.IsActive, state.AdminApproved, state.LastLoginAt
		}
	}

	user, err := userRepo.FindByID(userID)
	if err != nil || user == nil {
		return false, false, nil
	}

	if userCache != nil {
		state := &cache.CachedUserState{
			IsActive:      user.IsActive,
			AdminApproved: user.AdminApproved,
			LastLoginAt:   user.LastLoginAt,
		}
		_ = userCache.Set(context.Background(), userID, state)
	}

	return user.IsActive, user.AdminApproved, user.LastLoginAt
}
