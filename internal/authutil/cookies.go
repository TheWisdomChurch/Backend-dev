package authutil

import (
	"net/http"
	"os"
	"strings"
	"time"
)

// Cookie names used for auth session/token state.
const (
	AuthTokenCookieName    = "auth_token"
	LastActivityCookieName = "last_activity"
	DeviceIDCookieName     = "device_id"
	RefreshTokenCookieName = "refresh_token"
	OAuthStateCookieName   = "oauth_google_state"
)

var (
	authCookieClearPathCandidates = []string{
		"/",
		"/api",
		"/api/v1",
		"/api/v1/auth",
	}

	authCookieKnownDomainCandidates = []string{
		"",
		".wisdomchurchhq.org",
		"wisdomchurchhq.org",
		"api.wisdomchurchhq.org",
		"admin.wisdomchurchhq.org",
	}
)

// NormalizeConfiguredCookieDomain turns a configured host/URL value into a
// cookie-Domain-safe value, or "" when a Domain attribute shouldn't be set at
// all (localhost, host:port, empty) — browsers reject those for Domain and
// local development would otherwise break.
func NormalizeConfiguredCookieDomain(raw string) string {
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

	value = strings.TrimSpace(strings.ToLower(value))

	if value == "" || value == "localhost" || strings.Contains(value, ":") {
		return ""
	}

	if !strings.HasPrefix(value, ".") {
		value = "." + value
	}

	return value
}

// ConfiguredAuthCookieDomain resolves the cookie Domain attribute from the
// first of AUTH_COOKIE_DOMAIN / SESSION_COOKIE_DOMAIN / COOKIE_DOMAIN that's set.
func ConfiguredAuthCookieDomain() string {
	for _, key := range []string{
		"AUTH_COOKIE_DOMAIN",
		"SESSION_COOKIE_DOMAIN",
		"COOKIE_DOMAIN",
	} {
		if value := NormalizeConfiguredCookieDomain(os.Getenv(key)); value != "" {
			return value
		}
	}

	return ""
}

// AuthCookieClearDomains returns every domain variant auth cookies may have
// been set under historically, so a clear/logout pass reaches all of them.
func AuthCookieClearDomains() []string {
	candidates := make([]string, 0, len(authCookieKnownDomainCandidates)+4)

	candidates = append(candidates, authCookieKnownDomainCandidates...)
	candidates = append(candidates,
		ConfiguredAuthCookieDomain(),
		NormalizeConfiguredCookieDomain(os.Getenv("AUTH_COOKIE_DOMAIN")),
		NormalizeConfiguredCookieDomain(os.Getenv("SESSION_COOKIE_DOMAIN")),
		NormalizeConfiguredCookieDomain(os.Getenv("COOKIE_DOMAIN")),
	)

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

func expireCookie(w http.ResponseWriter, name string, domain string, cookiePath string, secure bool, sameSite http.SameSite) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     cookiePath,
		Domain:   domain,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0).UTC(),
		Secure:   secure,
		HttpOnly: true,
		SameSite: sameSite,
	})
}

// ExpireAuthCookie expires a cookie across every known auth-cookie path.
func ExpireAuthCookie(w http.ResponseWriter, name string, domain string, secure bool, sameSite http.SameSite) {
	for _, cookiePath := range authCookieClearPathCandidates {
		expireCookie(w, name, domain, cookiePath, secure, sameSite)
	}
}

// SetHTTPOnlyCookie writes an HttpOnly cookie with Path "/".
func SetHTTPOnlyCookie(w http.ResponseWriter, name string, value string, domain string, maxAge int, expires time.Time, secure bool, sameSite http.SameSite) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		Domain:   domain,
		MaxAge:   maxAge,
		Expires:  expires,
		Secure:   secure,
		HttpOnly: true,
		SameSite: sameSite,
	})
}

// LatestCookieValue returns the last non-empty cookie value for a name.
//
// This protects against cookie-domain migrations: browsers may send duplicate
// cookies when older host/path variants still exist.
func LatestCookieValue(r *http.Request, name string) (string, error) {
	if r == nil {
		return "", http.ErrNoCookie
	}

	values := make([]string, 0)

	for _, cookie := range r.Cookies() {
		if cookie == nil || cookie.Name != name {
			continue
		}

		value := strings.TrimSpace(cookie.Value)
		if value == "" {
			continue
		}

		values = append(values, value)
	}

	if len(values) == 0 {
		return "", http.ErrNoCookie
	}

	return values[len(values)-1], nil
}

// CookieJar centralizes HttpOnly auth-cookie read/write/clear mechanics — the
// SameSite/Secure/domain-migration details a handler shouldn't have to repeat.
//
// CSRF-cookie clearing is intentionally left to the caller (via the
// csrfCookieName parameter on ClearAuth) rather than baked in here, so this
// package doesn't need to depend on internal/middleware.
type CookieJar struct {
	Secure                       bool
	SessionIdleTimeout           time.Duration
	RememberedSessionIdleTimeout time.Duration
	RememberMeTTL                time.Duration
}

func NewCookieJar(secure bool, sessionIdleTimeout, rememberedSessionIdleTimeout, rememberMeTTL time.Duration) *CookieJar {
	return &CookieJar{
		Secure:                       secure,
		SessionIdleTimeout:           sessionIdleTimeout,
		RememberedSessionIdleTimeout: rememberedSessionIdleTimeout,
		RememberMeTTL:                rememberMeTTL,
	}
}

// SameSite returns SameSite=None when cookies are Secure (production, shared
// across subdomains), or SameSite=Lax for local/dev HTTP, where browsers
// reject SameSite=None without Secure.
func (j *CookieJar) SameSite() http.SameSite {
	if j.Secure {
		return http.SameSiteNoneMode
	}
	return http.SameSiteLaxMode
}

// ClearSessionVariants removes stale host/domain/path variants of the auth
// session cookies, so browsers don't send duplicates alongside a fresh write.
func (j *CookieJar) ClearSessionVariants(w http.ResponseWriter) {
	sameSite := j.SameSite()
	for _, domain := range AuthCookieClearDomains() {
		ExpireAuthCookie(w, AuthTokenCookieName, domain, j.Secure, sameSite)
		ExpireAuthCookie(w, LastActivityCookieName, domain, j.Secure, sameSite)
	}
}

// SetAuth writes the auth_token + last_activity cookies, clearing stale
// variants first.
func (j *CookieJar) SetAuth(w http.ResponseWriter, token string, rememberMe bool) {
	j.ClearSessionVariants(w)

	now := time.Now().UTC()
	maxAge := 0
	expires := time.Time{}
	activityMaxAge := 0
	activityExpires := time.Time{}

	if rememberMe {
		maxAge = int(j.RememberMeTTL / time.Second)
		expires = now.Add(j.RememberMeTTL)

		activityMaxAge = int(j.RememberedSessionIdleTimeout / time.Second)
		activityExpires = now.Add(j.RememberedSessionIdleTimeout)
	}

	sameSite := j.SameSite()
	domain := ConfiguredAuthCookieDomain()

	// In production, AUTH_COOKIE_DOMAIN should be ".wisdomchurchhq.org" if the
	// session must be shared across admin/api subdomains.
	SetHTTPOnlyCookie(w, AuthTokenCookieName, token, domain, maxAge, expires, j.Secure, sameSite)
	SetHTTPOnlyCookie(w, LastActivityCookieName, now.Format(time.RFC3339), domain, activityMaxAge, activityExpires, j.Secure, sameSite)
}

// ClearAuth removes the auth_token, last_activity, the given CSRF cookie, and
// the OAuth state cookie, across every known domain.
func (j *CookieJar) ClearAuth(w http.ResponseWriter, csrfCookieName string) {
	sameSite := j.SameSite()
	for _, domain := range AuthCookieClearDomains() {
		ExpireAuthCookie(w, AuthTokenCookieName, domain, j.Secure, sameSite)
		ExpireAuthCookie(w, LastActivityCookieName, domain, j.Secure, sameSite)
		ExpireAuthCookie(w, csrfCookieName, domain, j.Secure, sameSite)
		ExpireAuthCookie(w, OAuthStateCookieName, domain, j.Secure, http.SameSiteLaxMode)
	}
}

// ExpireRefreshToken removes the refresh_token cookie across every known domain.
func (j *CookieJar) ExpireRefreshToken(w http.ResponseWriter) {
	sameSite := j.SameSite()
	for _, domain := range AuthCookieClearDomains() {
		ExpireAuthCookie(w, RefreshTokenCookieName, domain, j.Secure, sameSite)
	}
}

// SetRefreshToken writes the refresh_token HttpOnly cookie.
func (j *CookieJar) SetRefreshToken(w http.ResponseWriter, rawValue string, ttl time.Duration) {
	sameSite := j.SameSite()
	domain := ConfiguredAuthCookieDomain()
	maxAge := int(ttl / time.Second)
	expires := time.Now().Add(ttl)
	SetHTTPOnlyCookie(w, RefreshTokenCookieName, rawValue, domain, maxAge, expires, j.Secure, sameSite)
}

// SetOAuthState writes the oauth_google_state cookie (10-minute TTL).
func (j *CookieJar) SetOAuthState(w http.ResponseWriter, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     OAuthStateCookieName,
		Value:    value,
		Path:     "/",
		Domain:   ConfiguredAuthCookieDomain(),
		MaxAge:   int((10 * time.Minute) / time.Second),
		Expires:  time.Now().UTC().Add(10 * time.Minute),
		Secure:   j.Secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearOAuthState clears the oauth_google_state cookie across every known domain.
func (j *CookieJar) ClearOAuthState(w http.ResponseWriter) {
	for _, domain := range AuthCookieClearDomains() {
		ExpireAuthCookie(w, OAuthStateCookieName, domain, j.Secure, http.SameSiteLaxMode)
	}
}
