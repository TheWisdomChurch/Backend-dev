package middleware

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	DefaultCSRFCookieName = "csrf_secret"
	DefaultCSRFHeaderName = "X-CSRF-Token"
)

type CSRFOptions struct {
	SecretKey string
	Secure    bool

	CookieName string
	HeaderName string
	CookieTTL  time.Duration
}

type CSRFProtector struct {
	secretKey  []byte
	secure     bool
	cookieName string
	headerName string
	cookieTTL  time.Duration
}

func NewCSRFProtector(opts CSRFOptions) *CSRFProtector {
	cookieName := strings.TrimSpace(opts.CookieName)
	if cookieName == "" {
		cookieName = DefaultCSRFCookieName
	}

	headerName := strings.TrimSpace(opts.HeaderName)
	if headerName == "" {
		headerName = DefaultCSRFHeaderName
	}

	if opts.CookieTTL <= 0 {
		opts.CookieTTL = 12 * time.Hour
	}

	return &CSRFProtector{
		secretKey:  []byte(strings.TrimSpace(opts.SecretKey)),
		secure:     opts.Secure,
		cookieName: cookieName,
		headerName: headerName,
		cookieTTL:  opts.CookieTTL,
	}
}

func (p *CSRFProtector) CookieName() string {
	if p == nil {
		return DefaultCSRFCookieName
	}
	return p.cookieName
}

func (p *CSRFProtector) HeaderName() string {
	if p == nil {
		return DefaultCSRFHeaderName
	}
	return p.headerName
}

func (p *CSRFProtector) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if p == nil || len(p.secretKey) == 0 {
			c.Next()
			return
		}

		secret, token, err := p.ensureSecretAndToken(c)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"status":     "error",
				"error":      "Forbidden",
				"message":    "CSRF protection is not available",
				"statusCode": http.StatusForbidden,
			})
			return
		}

		c.Set("csrf_token", token)
		c.Set("csrf_header", p.headerName)
		c.Header(p.headerName, token)

		if isSafeHTTPMethod(c.Request.Method) {
			c.Next()
			return
		}

		presented := strings.TrimSpace(c.GetHeader(p.headerName))
		// verify() parses the nonce from the presented token and recomputes the MAC
		// against the cookie secret — so every request can have a unique nonce while
		// still being verifiable without server-side state.
		if presented == "" || !p.verify(secret, presented) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"status":     "error",
				"error":      "Forbidden",
				"message":    "Invalid or missing CSRF token",
				"statusCode": http.StatusForbidden,
			})
			return
		}

		c.Next()
	}
}

// EnsureToken returns a per-request CSRF token for use in API responses or tests.
func (p *CSRFProtector) EnsureToken(c *gin.Context) (string, error) {
	_, token, err := p.ensureSecretAndToken(c)
	return token, err
}

// ensureSecretAndToken returns both the raw cookie secret and the signed nonce-bearing token.
func (p *CSRFProtector) ensureSecretAndToken(c *gin.Context) (secret, token string, err error) {
	if p == nil || len(p.secretKey) == 0 {
		return "", "", http.ErrNoCookie
	}

	secret, err = LatestCookieValue(c.Request, p.cookieName)
	if err != nil || strings.TrimSpace(secret) == "" {
		secret, err = generateCSRFSecret()
		if err != nil {
			return "", "", err
		}
		p.setCSRFCookie(c, secret)
	}

	token, err = p.sign(secret)
	return secret, token, err
}

func (p *CSRFProtector) setCSRFCookie(c *gin.Context, secret string) {
	expires := time.Now().UTC().Add(p.cookieTTL)

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     p.cookieName,
		Value:    strings.TrimSpace(secret),
		Path:     "/",
		Domain:   configuredSessionCookieDomain(),
		MaxAge:   int(p.cookieTTL / time.Second),
		Expires:  expires,
		Secure:   p.secure,
		HttpOnly: true,
		SameSite: sameSiteForEnv(p.secure),
	})
}

// sign returns a per-request token of the form "<nonce>.<hmac>" where the nonce is
// a fresh 16-byte random value encoded in base64. Because the nonce changes on every
// call the token is never the same across requests, eliminating BREACH-style
// compression attacks against a static CSRF token.
func (p *CSRFProtector) sign(secret string) (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	nonceB64 := base64.RawURLEncoding.EncodeToString(nonce)

	mac := hmac.New(sha256.New, p.secretKey)
	_, _ = mac.Write([]byte(strings.TrimSpace(secret) + ":" + nonceB64))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return nonceB64 + "." + sig, nil
}

// verify checks that the presented token is a valid signature over the cookie secret.
// It parses the nonce from the presented token so the verification is stateless.
func (p *CSRFProtector) verify(secret, presented string) bool {
	parts := strings.SplitN(strings.TrimSpace(presented), ".", 2)
	if len(parts) != 2 {
		return false
	}
	nonceB64 := parts[0]

	mac := hmac.New(sha256.New, p.secretKey)
	_, _ = mac.Write([]byte(strings.TrimSpace(secret) + ":" + nonceB64))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return subtle.ConstantTimeCompare([]byte(parts[1]), []byte(expected)) == 1
}

func ClearCSRFCookie(c *gin.Context, secure bool, cookieName string) {
	name := strings.TrimSpace(cookieName)
	if name == "" {
		name = DefaultCSRFCookieName
	}

	sameSite := sameSiteForEnv(secure)
	paths := []string{"/", "/api", "/api/v1", "/api/v1/auth"}

	for _, domain := range sessionCookieClearDomains() {
		for _, cookiePath := range paths {
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
}

func generateCSRFSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func isSafeHTTPMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}
