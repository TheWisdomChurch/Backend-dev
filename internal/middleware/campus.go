package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	CampusIDKey    = "campus_id"
	CampusIDHeader = "X-Campus-ID"
)

// CampusContext extracts the X-Campus-ID header and stores it in the Gin context.
// If the header is present and non-empty, downstream handlers can retrieve it
// with c.GetString(middleware.CampusIDKey). The middleware is intentionally
// permissive — it does not reject requests without the header, because many
// endpoints are campus-agnostic.
func CampusContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		campusID := strings.TrimSpace(c.GetHeader(CampusIDHeader))
		if campusID != "" {
			c.Set(CampusIDKey, campusID)
		}
		c.Next()
	}
}

// RequireCampus is a stricter variant that rejects the request with 400 when
// X-Campus-ID is missing or empty. Use it on routes where a campus scope is
// mandatory (e.g. campus-specific attendance or giving reports).
func RequireCampus() gin.HandlerFunc {
	return func(c *gin.Context) {
		campusID := strings.TrimSpace(c.GetHeader(CampusIDHeader))
		if campusID == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "X-Campus-ID header is required for this endpoint",
			})
			return
		}
		c.Set(CampusIDKey, campusID)
		c.Next()
	}
}

// GetCampusID is a convenience helper for handlers to read the campus ID
// stored by CampusContext or RequireCampus.
func GetCampusID(c *gin.Context) string {
	v, _ := c.Get(CampusIDKey)
	id, _ := v.(string)
	return id
}
