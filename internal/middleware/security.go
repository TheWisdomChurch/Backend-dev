package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/email"
)

func SecurityHeaders(isProduction bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "0")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Cross-Origin-Opener-Policy", "same-origin")
		// Cross-Origin-Resource-Policy: same-origin blocks any other origin
		// from loading this response — correct default for API/JSON
		// responses, but it silently breaks publicly embeddable assets like
		// the email logo: webmail clients (Gmail, Outlook, etc.) render HTML
		// emails from their OWN origin, so an <img> pointing at this API is
		// always a cross-origin load from the browser's point of view. That
		// was the actual reason the logo never rendered in sent emails, even
		// though the endpoint itself served correct bytes.
		if isEmbeddablePublicAsset(c.Request.URL.Path) {
			c.Header("Cross-Origin-Resource-Policy", "cross-origin")
		} else {
			c.Header("Cross-Origin-Resource-Policy", "same-origin")
		}
		c.Header("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		if isProduction {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		path := c.Request.URL.Path
		if isPresentationPage(path) {
			c.Header("Content-Security-Policy",
				"default-src 'self'; "+
					"frame-ancestors 'none'; "+
					"base-uri 'self'; "+
					"form-action 'self'; "+
					"connect-src 'self'; "+
					"script-src 'self' 'unsafe-inline'; "+
					"style-src 'self' 'unsafe-inline'; "+
					"img-src 'self' data: https:; "+
					"font-src 'self' data: https:;")
		} else {
			c.Header("Content-Security-Policy",
				"default-src 'none'; "+
					"frame-ancestors 'none'; "+
					"base-uri 'self';")
		}

		c.Next()
	}
}

func isPresentationPage(path string) bool {
	return strings.HasPrefix(path, "/forms/") ||
		strings.HasPrefix(path, "/form/") ||
		strings.HasPrefix(path, "/reports/forms/") ||
		strings.HasPrefix(path, "/api/v1/auth/oauth/google/")
}

// isEmbeddablePublicAsset matches routes that serve static, unauthenticated
// binary assets meant to be loaded by third-party origins (e.g. an <img> tag
// inside an email rendered on mail.google.com). Nothing here ever depends on
// a session/cookie, so relaxing Cross-Origin-Resource-Policy for just these
// paths carries none of the risk it would for an API/JSON route.
func isEmbeddablePublicAsset(path string) bool {
	return path == email.LogoAssetPath
}
