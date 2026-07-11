package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/authutil"
)

const (
	sessionAuthCookieName         = "auth_token"
	sessionLastActivityCookieName = "last_activity"
)

// Cookie-domain resolution used to be duplicated here with a subtly different
// implementation (no lowercasing, a narrower clear-domain list) than
// internal/authutil/cookies.go, which is the package that actually writes
// these cookies at login. Two independently-maintained copies of "which
// domain does this cookie live under" is exactly the kind of drift that
// causes a cookie set at login to not be recognized a moment later — so this
// now delegates to the single canonical implementation instead.
func configuredSessionCookieDomain() string {
	return authutil.ConfiguredAuthCookieDomain()
}

func sessionCookieClearDomains() []string {
	return authutil.AuthCookieClearDomains()
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
