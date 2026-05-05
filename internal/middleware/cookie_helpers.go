package middleware

import (
	"net/http"
	"strings"
	"time"
)

// CookieValues returns all non-empty cookie values with the given name.
//
// Browsers can send duplicate cookies when a backend previously used different
// Domain or Path attributes. Auth code must not trust only the first cookie.
func CookieValues(r *http.Request, name string) []string {
	if r == nil {
		return nil
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

	return values
}

// LatestCookieValue returns the last non-empty cookie value for a duplicated
// cookie name. This is safer during cookie-domain migrations because the newer
// cookie is commonly sent later in the Cookie header.
func LatestCookieValue(r *http.Request, name string) (string, error) {
	values := CookieValues(r, name)
	if len(values) == 0 {
		return "", http.ErrNoCookie
	}

	return values[len(values)-1], nil
}

// LatestHTTPCookie returns a cookie object for the latest non-empty cookie value.
func LatestHTTPCookie(r *http.Request, name string) (*http.Cookie, error) {
	value, err := LatestCookieValue(r, name)
	if err != nil {
		return nil, err
	}

	return &http.Cookie{
		Name:  name,
		Value: value,
	}, nil
}

// LatestRFC3339Cookie returns the cookie with the newest RFC3339 timestamp.
// This prevents a stale duplicated last_activity cookie from expiring a valid session.
func LatestRFC3339Cookie(r *http.Request, name string) (*http.Cookie, error) {
	if r == nil {
		return nil, http.ErrNoCookie
	}

	var selected *http.Cookie
	var selectedAt time.Time

	for _, cookie := range r.Cookies() {
		if cookie == nil || cookie.Name != name {
			continue
		}

		value := strings.TrimSpace(cookie.Value)
		if value == "" {
			continue
		}

		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			continue
		}

		if selected == nil || parsed.After(selectedAt) {
			clone := *cookie
			clone.Value = value
			selected = &clone
			selectedAt = parsed
		}
	}

	if selected != nil {
		return selected, nil
	}

	return LatestHTTPCookie(r, name)
}
