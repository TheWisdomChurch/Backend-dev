package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const (
	JWTIssuer   = "wisdom-house-backend"
	JWTAudience = "wisdom-house-clients"
)

const authTokenCookieName = "auth_token"

type AccessClaims struct {
	UserID                    string `json:"user_id"`
	Email                     string `json:"email"`
	Role                      string `json:"role"`
	RememberMe                bool   `json:"remember_me"`
	SessionIdleTimeoutSeconds int64  `json:"session_idle_timeout_seconds"`
	AuthMethod                string `json:"auth_method"`
	jwt.RegisteredClaims
}

func (c AccessClaims) Validate() error {
	if strings.TrimSpace(c.UserID) == "" {
		return errors.New("missing user_id")
	}
	if strings.TrimSpace(c.Email) == "" {
		return errors.New("missing email")
	}
	if strings.TrimSpace(c.Role) == "" {
		return errors.New("missing role")
	}
	return nil
}

func unauthorized(c *gin.Context, message string) {
	c.JSON(http.StatusUnauthorized, gin.H{
		"status":     "error",
		"error":      "Unauthorized",
		"message":    message,
		"statusCode": http.StatusUnauthorized,
	})
	c.Abort()
}

func validateAccessToken(rawToken string, jwtSecret string) (*AccessClaims, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return nil, fmt.Errorf("missing auth token")
	}
	if strings.TrimSpace(jwtSecret) == "" {
		return nil, fmt.Errorf("jwt secret not configured")
	}

	claims := &AccessClaims{}
	token, err := jwt.ParseWithClaims(
		rawToken,
		claims,
		func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(jwtSecret), nil
		},
		jwt.WithIssuer(JWTIssuer),
		jwt.WithAudience(JWTAudience),
	)
	if err != nil {
		return nil, err
	}
	if token == nil || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	now := time.Now().UTC()
	if claims.ExpiresAt != nil && now.After(claims.ExpiresAt.Time) {
		return nil, fmt.Errorf("token expired")
	}
	if claims.NotBefore != nil && now.Before(claims.NotBefore.Time) {
		return nil, fmt.Errorf("token not active yet")
	}
	if claims.IssuedAt != nil && claims.IssuedAt.Time.After(now.Add(5*time.Minute)) {
		return nil, fmt.Errorf("token issued in the future")
	}
	if err := claims.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(claims.Subject) != "" && claims.Subject != claims.UserID {
		return nil, fmt.Errorf("token subject mismatch")
	}

	claims.UserID = strings.TrimSpace(claims.UserID)
	claims.Email = strings.TrimSpace(claims.Email)
	claims.Role = strings.TrimSpace(claims.Role)
	claims.AuthMethod = strings.TrimSpace(claims.AuthMethod)

	return claims, nil
}

func bearerTokenFromHeader(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}

	header := strings.TrimSpace(c.GetHeader("Authorization"))
	if header == "" {
		return ""
	}

	const prefix = "Bearer "
	if !strings.HasPrefix(strings.ToLower(header), strings.ToLower(prefix)) {
		return ""
	}

	return strings.TrimSpace(header[len(prefix):])
}

func candidateAccessTokens(c *gin.Context) []string {
	seen := make(map[string]bool)
	tokens := make([]string, 0)

	appendToken := func(token string) {
		token = strings.TrimSpace(token)
		if token == "" || seen[token] {
			return
		}
		seen[token] = true
		tokens = append(tokens, token)
	}

	if c != nil && c.Request != nil {
		for _, token := range CookieValues(c.Request, authTokenCookieName) {
			appendToken(token)
		}
	}

	appendToken(bearerTokenFromHeader(c))

	return tokens
}

func parseCookieToken(c *gin.Context, jwtSecret string) (*AccessClaims, error) {
	tokens := candidateAccessTokens(c)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("missing auth cookie")
	}

	var lastErr error

	// Try the newest browser cookie first. During Domain/Path migrations, stale
	// cookies can remain in the browser and appear before the newer cookie.
	for i := len(tokens) - 1; i >= 0; i-- {
		claims, err := validateAccessToken(tokens[i], jwtSecret)
		if err == nil {
			return claims, nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, lastErr
	}

	return nil, fmt.Errorf("invalid token")
}

func setAccessClaimsContext(c *gin.Context, claims *AccessClaims) {
	if c == nil || claims == nil {
		return
	}

	c.Set("user_id", claims.UserID)
	c.Set("email", claims.Email)
	c.Set("role", claims.Role)
	c.Set("remember_me", claims.RememberMe)
	c.Set("session_idle_timeout_seconds", claims.SessionIdleTimeoutSeconds)
	c.Set("auth_method", claims.AuthMethod)
	c.Set("auth_claims", claims)
}

func AuthMiddleware(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, err := parseCookieToken(c, jwtSecret)
		if err != nil {
			unauthorized(c, "Invalid or expired token")
			return
		}

		setAccessClaimsContext(c, claims)
		c.Next()
	}
}

func OptionalAuthMiddleware(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, err := parseCookieToken(c, jwtSecret)
		if err == nil && claims != nil {
			setAccessClaimsContext(c, claims)
		}
		c.Next()
	}
}

func GetUserIDFromContext(c *gin.Context) (string, bool) {
	if c == nil {
		return "", false
	}

	userID, exists := c.Get("user_id")
	if !exists {
		return "", false
	}

	id, ok := userID.(string)
	id = strings.TrimSpace(id)
	return id, ok && id != ""
}
