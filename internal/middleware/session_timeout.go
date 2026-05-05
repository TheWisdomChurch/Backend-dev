package middleware

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func normalizeSessionCookieDomain(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}

	value = strings.TrimPrefix(value, "https://")
	value = strings.TrimPrefix(value, "http://")
	value = strings.Trim(value, " /")

	if slash := strings.Index(value, "/"); slash >= 0 {
		value = value[:slash]
	}

	value = strings.TrimSpace(value)

	if value == "" || value == "localhost" || strings.Contains(value, ":") {
		return ""
	}

	if !strings.HasPrefix(value, ".") {
		value = "." + value
	}

	return value
}

func configuredSessionCookieDomain() string {
	for _, key := range []string{
		"AUTH_COOKIE_DOMAIN",
		"SESSION_COOKIE_DOMAIN",
		"COOKIE_DOMAIN",
	} {
		if value := normalizeSessionCookieDomain(os.Getenv(key)); value != "" {
			return value
		}
	}

	return ""
}

func sessionCookieClearDomains() []string {
	domain := configuredSessionCookieDomain()
	if domain == "" {
		return []string{""}
	}

	return []string{"", domain}
}

func expireSessionCookie(c *gin.Context, name string, domain string, secure bool, sameSite http.SameSite) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		Domain:   domain,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		Secure:   secure,
		HttpOnly: true,
		SameSite: sameSite,
	})
}

// SessionTimeout enforces inactivity logout based on a last-activity cookie.
func SessionTimeout(defaultTimeout, rememberedTimeout time.Duration, secure bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		now := time.Now().UTC()
		sameSite := sameSiteForEnv(secure)
		cookieDomain := configuredSessionCookieDomain()
		persistentActivityCookie := false

		expireAuth := func() {
			for _, domain := range sessionCookieClearDomains() {
				expireSessionCookie(c, "auth_token", domain, secure, sameSite)
				expireSessionCookie(c, "last_activity", domain, secure, sameSite)
			}
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
		if err != nil || strings.TrimSpace(lastActivityCookie.Value) == "" {
			expireAuth()
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"status":     "error",
				"error":      "Unauthorized",
				"message":    "Session expired due to inactivity",
				"statusCode": http.StatusUnauthorized,
			})
			return
		}

		lastActivity, parseErr := time.Parse(time.RFC3339, lastActivityCookie.Value)
		if parseErr != nil || now.Sub(lastActivity) > timeout {
			expireAuth()
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"status":     "error",
				"error":      "Unauthorized",
				"message":    "Session expired due to inactivity",
				"statusCode": http.StatusUnauthorized,
			})
			return
		}

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
			Domain:   cookieDomain,
			MaxAge:   maxAge,
			Expires:  expires,
			Secure:   secure,
			HttpOnly: true,
			SameSite: sameSite,
		})

		c.Next()
	}
}