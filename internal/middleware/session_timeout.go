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
		sameSite := sameSiteForEnv(secure)

		expireAuth := func() {
			// expire auth + activity cookies
			http.SetCookie(c.Writer, &http.Cookie{
				Name:     "auth_token",
				Value:    "",
				Path:     "/",
				MaxAge:   -1,
				Expires:  time.Unix(0, 0),
				Secure:   secure,
				HttpOnly: true,
				SameSite: sameSite,
			})
			http.SetCookie(c.Writer, &http.Cookie{
				Name:     "last_activity",
				Value:    "",
				Path:     "/",
				MaxAge:   -1,
				Expires:  time.Unix(0, 0),
				Secure:   secure,
				HttpOnly: true,
				SameSite: sameSite,
			})
		}

		lastActivityCookie, err := c.Request.Cookie("last_activity")
		if err != nil || lastActivityCookie.Value == "" {
			expireAuth()
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"status":  "error",
				"message": "Session expired due to inactivity",
			})
			return
		}

		lastActivity, parseErr := time.Parse(time.RFC3339, lastActivityCookie.Value)
		if parseErr != nil || now.Sub(lastActivity) > timeout {
			expireAuth()
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"status":  "error",
				"message": "Session expired due to inactivity",
			})
			return
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
			SameSite: sameSite,
		})

		c.Next()
	}
}
