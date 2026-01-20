package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

/* ============================================================================
   Claims
============================================================================ */

type AccessClaims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// Validate enforces required fields beyond standard exp/nbf checks.
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

/* ============================================================================
   Helpers
============================================================================ */

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
	if err != nil {
		return nil, fmt.Errorf("missing auth cookie")
	}

	claims := &AccessClaims{}

	// ParseWithClaims also validates signature. We validate exp/nbf manually below.
	token, err := jwt.ParseWithClaims(cookie, claims, func(token *jwt.Token) (interface{}, error) {
		// Enforce HMAC SHA
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		if jwtSecret == "" {
			return nil, fmt.Errorf("jwt secret not configured")
		}
		return []byte(jwtSecret), nil
	})

	if err != nil {
		return nil, err
	}
	if token == nil || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	// Expiration validation: RegisteredClaims provides helpers.
	// jwt/v5 supports claims validation via jwt.Validator, but explicit check is fine.
	now := time.Now()

	// If ExpiresAt exists, enforce it.
	if claims.ExpiresAt != nil && now.After(claims.ExpiresAt.Time) {
		return nil, fmt.Errorf("token expired")
	}
	// If NotBefore exists, enforce it.
	if claims.NotBefore != nil && now.Before(claims.NotBefore.Time) {
		return nil, fmt.Errorf("token not active yet")
	}

	// Custom required fields
	if err := claims.Validate(); err != nil {
		return nil, err
	}

	return claims, nil
}

/* ============================================================================
   Middleware
============================================================================ */

// AuthMiddleware validates JWT from cookie and injects identity into context
func AuthMiddleware(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, err := parseCookieToken(c, jwtSecret)
		if err != nil {
			unauthorized(c, "Invalid or expired token")
			return
		}

		// Set claims in context
		c.Set("user_id", claims.UserID)
		c.Set("email", claims.Email)
		c.Set("role", claims.Role)

		c.Next()
	}
}

// OptionalAuthMiddleware tries to extract user from token but doesn't fail if missing/invalid
func OptionalAuthMiddleware(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, err := parseCookieToken(c, jwtSecret)
		if err == nil && claims != nil {
			c.Set("user_id", claims.UserID)
			c.Set("email", claims.Email)
			c.Set("role", claims.Role)
		}
		c.Next()
	}
}

// GetUserIDFromContext retrieves user_id from context
func GetUserIDFromContext(c *gin.Context) (string, bool) {
	userID, exists := c.Get("user_id")
	if !exists {
		return "", false
	}
	userIDStr, ok := userID.(string)
	return userIDStr, ok
}
