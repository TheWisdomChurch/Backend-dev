package middleware

import "net/http"

// sameSiteForEnv aligns cookie SameSite behavior with secure deployment needs.
// Secure cookies require SameSite=None for cross-site use (e.g. admin portal).
func sameSiteForEnv(secure bool) http.SameSite {
	if secure {
		return http.SameSiteNoneMode
	}
	return http.SameSiteLaxMode
}
