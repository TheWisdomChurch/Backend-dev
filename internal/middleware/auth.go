package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const (
	JWTIssuer   = "wisdom-house-backend"
	JWTAudience = "wisdom-house-clients"
)

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
	if c.UserID == "" {
		return errors.New("missing user_id")
	}
	if c.Email == "" {
		return errors.New("missing email")
	}
	if c.Role == "" {
		return errors.New("missing role")
	}
	return nil
}

func unauthorized(c *gin.Context, message string) {
	c.JSON(http.StatusUnauthorized, gin.H{
		"error":      "Unauthorized",
		"message":    message,
		"statusCode": http.StatusUnauthorized,
	})
	c.Abort()
}

func parseCookieToken(c *gin.Context, jwtSecret string) (*AccessClaims, error) {
	cookie, err := c.Cookie("auth_token")
	if err != nil || cookie == "" {
		return nil, fmt.Errorf("missing auth cookie")
	}

	claims := &AccessClaims{}
	token, err := jwt.ParseWithClaims(
		cookie,
		claims,
		func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			if jwtSecret == "" {
				return nil, fmt.Errorf("jwt secret not configured")
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

	now := time.Now()
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
	if claims.Subject != "" && claims.Subject != claims.UserID {
		return nil, fmt.Errorf("token subject mismatch")
	}

	return claims, nil
}

func AuthMiddleware(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, err := parseCookieToken(c, jwtSecret)
		if err != nil {
			unauthorized(c, "Invalid or expired token")
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("email", claims.Email)
		c.Set("role", claims.Role)
		c.Set("remember_me", claims.RememberMe)
		c.Set("session_idle_timeout_seconds", claims.SessionIdleTimeoutSeconds)
		c.Set("auth_method", claims.AuthMethod)
		c.Next()
	}
}

func OptionalAuthMiddleware(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, err := parseCookieToken(c, jwtSecret)
		if err == nil && claims != nil {
			c.Set("user_id", claims.UserID)
			c.Set("email", claims.Email)
			c.Set("role", claims.Role)
			c.Set("remember_me", claims.RememberMe)
			c.Set("session_idle_timeout_seconds", claims.SessionIdleTimeoutSeconds)
			c.Set("auth_method", claims.AuthMethod)
		}
		c.Next()
	}
}

func GetUserIDFromContext(c *gin.Context) (string, bool) {
	userID, exists := c.Get("user_id")
	if !exists {
		return "", false
	}
	id, ok := userID.(string)
	return id, ok && id != ""
}
