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
	return p.cookieName
}

func (p *CSRFProtector) HeaderName() string {
	return p.headerName
}

func (p *CSRFProtector) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if p == nil || len(p.secretKey) == 0 {
			c.Next()
			return
		}

		token, err := p.EnsureToken(c)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
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
		if presented == "" || subtle.ConstantTimeCompare([]byte(presented), []byte(token)) != 1 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":      "Forbidden",
				"message":    "Invalid or missing CSRF token",
				"statusCode": http.StatusForbidden,
			})
			return
		}

		c.Next()
	}
}

func (p *CSRFProtector) EnsureToken(c *gin.Context) (string, error) {
	if p == nil || len(p.secretKey) == 0 {
		return "", http.ErrNoCookie
	}

	secret, err := c.Cookie(p.cookieName)
	if err != nil || strings.TrimSpace(secret) == "" {
		secret, err = generateCSRFSecret()
		if err != nil {
			return "", err
		}
		p.setCSRFCookie(c, secret)
	}

	return p.sign(secret), nil
}

func (p *CSRFProtector) setCSRFCookie(c *gin.Context, secret string) {
	expires := time.Now().UTC().Add(p.cookieTTL)
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     p.cookieName,
		Value:    secret,
		Path:     "/",
		MaxAge:   int(p.cookieTTL / time.Second),
		Expires:  expires,
		Secure:   p.secure,
		HttpOnly: true,
		SameSite: sameSiteForEnv(p.secure),
	})
}

func (p *CSRFProtector) sign(secret string) string {
	mac := hmac.New(sha256.New, p.secretKey)
	_, _ = mac.Write([]byte(secret))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func ClearCSRFCookie(c *gin.Context, secure bool, cookieName string) {
	name := strings.TrimSpace(cookieName)
	if name == "" {
		name = DefaultCSRFCookieName
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		Secure:   secure,
		HttpOnly: true,
		SameSite: sameSiteForEnv(secure),
	})
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
