package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// DeviceFingerprint ensures each client has a stable, opaque device id cookie we can use for anomaly detection.
func DeviceFingerprint(secure bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		const cookieName = "device_id"
		if _, err := c.Request.Cookie(cookieName); err != nil {
			id := uuid.NewString()
			http.SetCookie(c.Writer, &http.Cookie{
				Name:     cookieName,
				Value:    id,
				Path:     "/",
				Expires:  time.Now().Add(365 * 24 * time.Hour),
				MaxAge:   365 * 24 * 3600,
				Secure:   secure,
				HttpOnly: false, // needs to be readable by frontend if desired; keep value opaque
				SameSite: http.SameSiteLaxMode,
			})
		}
		c.Next()
	}
}
