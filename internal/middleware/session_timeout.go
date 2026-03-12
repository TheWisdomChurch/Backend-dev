package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// SessionTimeout enforces inactivity logout based on a last-activity cookie.
func SessionTimeout(defaultTimeout, rememberedTimeout time.Duration, secure bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		now := time.Now().UTC()
		sameSite := sameSiteForEnv(secure)
		persistentActivityCookie := false

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

		timeout := defaultTimeout
		if rememberMe, _ := c.Get("remember_me"); rememberMe == true {
			timeout = rememberedTimeout
			persistentActivityCookie = true
		}
		if raw, exists := c.Get("session_idle_timeout_seconds"); exists {
			switch v := raw.(type) {
			case int64:
				if v > 0 {
					timeout = time.Duration(v) * time.Second
				}
			case int:
				if v > 0 {
					timeout = time.Duration(v) * time.Second
				}
			case float64:
				if v > 0 {
					timeout = time.Duration(v) * time.Second
				}
			}
		}
		if timeout <= 0 {
			timeout = defaultTimeout
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
		maxAge := 0
		expires := time.Time{}
		if persistentActivityCookie {
			maxAge = int(timeout.Seconds())
			expires = now.Add(timeout)
		}
		http.SetCookie(c.Writer, &http.Cookie{
			Name:     "last_activity",
			Value:    now.Format(time.RFC3339),
			Path:     "/",
			MaxAge:   maxAge,
			Expires:  expires,
			Secure:   secure,
			HttpOnly: true,
			SameSite: sameSite,
		})

		c.Next()
	}
}
