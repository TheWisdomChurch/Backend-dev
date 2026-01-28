package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// SessionTimeout enforces inactivity logout based on a last-activity cookie.
func SessionTimeout(timeout time.Duration, secure bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		now := time.Now().UTC()
		lastActivityCookie, err := c.Request.Cookie("last_activity")
		if err == nil {
			if t, parseErr := time.Parse(time.RFC3339, lastActivityCookie.Value); parseErr == nil {
				if now.Sub(t) > timeout {
					// expire auth + activity cookies
					http.SetCookie(c.Writer, &http.Cookie{
						Name:     "auth_token",
						Value:    "",
						Path:     "/",
						MaxAge:   -1,
						Expires:  time.Unix(0, 0),
						Secure:   secure,
						HttpOnly: true,
						SameSite: http.SameSiteLaxMode,
					})
					http.SetCookie(c.Writer, &http.Cookie{
						Name:     "last_activity",
						Value:    "",
						Path:     "/",
						MaxAge:   -1,
						Expires:  time.Unix(0, 0),
						Secure:   secure,
						HttpOnly: true,
						SameSite: http.SameSiteLaxMode,
					})
					c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
						"status":  "error",
						"message": "Session expired due to inactivity",
					})
					return
				}
			}
		}

		// refresh last activity
		http.SetCookie(c.Writer, &http.Cookie{
			Name:     "last_activity",
			Value:    now.Format(time.RFC3339),
			Path:     "/",
			MaxAge:   int(timeout.Seconds()),
			Expires:  now.Add(timeout),
			Secure:   secure,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})

		c.Next()
	}
}
