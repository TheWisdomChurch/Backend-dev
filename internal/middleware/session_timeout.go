package middleware

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	sessionAuthCookieName         = "auth_token"
	sessionLastActivityCookieName = "last_activity"
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

	// Do not set Domain for localhost, IP addresses, host:port values,
	// or empty values. Browsers reject those in local development.
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
	configuredDomain := configuredSessionCookieDomain()

	// Keep this list broader than the active cookie domain because production
	// deployments may have previously used host-only cookies, parent-domain
	// cookies, or explicit API-domain cookies. Clearing all known variants is
	// what stops duplicate Cookie headers from causing false 401 responses.
	candidates := []string{
		"",
		configuredDomain,
		".wisdomchurchhq.org",
		"wisdomchurchhq.org",
		"api.wisdomchurchhq.org",
	}

	seen := make(map[string]bool, len(candidates))
	domains := make([]string, 0, len(candidates))

	for _, domain := range candidates {
		domain = strings.TrimSpace(domain)
		if seen[domain] {
			continue
		}

		seen[domain] = true
		domains = append(domains, domain)
	}

	return domains
}

func sessionCookieClearPaths() []string {
	return []string{
		"/",
		"/api",
		"/api/v1",
		"/api/v1/auth",
	}
}

func expireSessionCookie(c *gin.Context, name string, domain string, secure bool, sameSite http.SameSite) {
	for _, cookiePath := range sessionCookieClearPaths() {
		http.SetCookie(c.Writer, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     cookiePath,
			Domain:   domain,
			MaxAge:   -1,
			Expires:  time.Unix(0, 0),
			Secure:   secure,
			HttpOnly: true,
			SameSite: sameSite,
		})
	}
}

// SessionTimeout enforces inactivity logout based on the newest last_activity cookie.
//
// This middleware intentionally reads the latest valid RFC3339 last_activity cookie
// instead of c.Request.Cookie("last_activity"). During cookie-domain/path migrations,
// browsers can send duplicated cookies, and reading only the first one can expire
// a valid session and cause protected routes such as GET /api/v1/auth/mfa to return 401.
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
				expireSessionCookie(c, sessionAuthCookieName, domain, secure, sameSite)
				expireSessionCookie(c, sessionLastActivityCookieName, domain, secure, sameSite)
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
		if timeout <= 0 {
			timeout = 30 * time.Minute
		}

		lastActivityCookie, err := LatestRFC3339Cookie(c.Request, sessionLastActivityCookieName)
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

		lastActivity, parseErr := time.Parse(time.RFC3339, strings.TrimSpace(lastActivityCookie.Value))
		if parseErr != nil || now.Sub(lastActivity.UTC()) > timeout {
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
			Name:     sessionLastActivityCookieName,
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
