// internal/middleware/auth.go
package middleware

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// AuthMiddleware validates JWT from cookie
func AuthMiddleware(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// DEBUG
		fmt.Printf("\n🔐 [AuthMiddleware] Checking authentication for: %s\n", c.Request.URL.Path)
		
		// Get cookie
		cookie, err := c.Cookie("auth_token")
		if err != nil {
			fmt.Println("❌ [AuthMiddleware] No auth cookie found")
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "Unauthorized",
				"message": "Authentication cookie is required",
			})
			c.Abort()
			return
		}
		
		fmt.Printf("🔐 [AuthMiddleware] Cookie (first 30 chars): %.30s...\n", cookie)

		// Parse and validate token
		token, err := jwt.Parse(cookie, func(token *jwt.Token) (interface{}, error) {
			// Validate signing method
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				method := token.Header["alg"]
				fmt.Printf("❌ [AuthMiddleware] Unexpected signing method: %v\n", method)
				return nil, fmt.Errorf("unexpected signing method: %v", method)
			}
			
			// DEBUG: Print the secret being used (first few chars)
			secretPreview := jwtSecret
			if len(secretPreview) > 10 {
				secretPreview = secretPreview[:10] + "..."
			}
			fmt.Printf("🔐 [AuthMiddleware] Using JWT secret: %s\n", secretPreview)
			
			return []byte(jwtSecret), nil
		})

		if err != nil {
			fmt.Printf("❌ [AuthMiddleware] Token parse error: %v\n", err)
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "Unauthorized",
				"message": "Invalid token: " + err.Error(),
			})
			c.Abort()
			return
		}

		if !token.Valid {
			fmt.Println("❌ [AuthMiddleware] Token is not valid")
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "Unauthorized",
				"message": "Invalid or expired token",
			})
			c.Abort()
			return
		}

		// Extract claims
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			fmt.Printf("✅ [AuthMiddleware] Token is VALID\n")
			fmt.Printf("✅ [AuthMiddleware] All claims: %v\n", claims)
			
			// Try different possible user ID keys
			var userID string
			var found bool
			
			// Try all possible claim names
			possibleKeys := []string{"user_id", "userID", "sub", "userId", "user"}
			for _, key := range possibleKeys {
				if val, ok := claims[key]; ok {
					if str, ok := val.(string); ok && str != "" {
						userID = str
						found = true
						fmt.Printf("✅ [AuthMiddleware] Found user_id as '%s': %s\n", key, userID)
						break
					}
				}
			}
			
			if !found {
				fmt.Printf("❌ [AuthMiddleware] No user ID found in claims. Available claims: %v\n", claims)
				c.JSON(http.StatusUnauthorized, gin.H{
					"error":   "Unauthorized",
					"message": "Token does not contain user ID",
				})
				c.Abort()
				return
			}
			
			// Set user_id in context
			c.Set("user_id", userID)
			fmt.Printf("✅ [AuthMiddleware] Set user_id in context: %s\n", userID)
			
			// Set other claims if available
			if email, ok := claims["email"].(string); ok {
				c.Set("email", email)
				fmt.Printf("✅ [AuthMiddleware] Set email in context: %s\n", email)
			}
			if role, ok := claims["role"].(string); ok {
				c.Set("role", role)
				fmt.Printf("✅ [AuthMiddleware] Set role in context: %s\n", role)
			}
			
			fmt.Println("✅ [AuthMiddleware] Authentication successful, proceeding...")
		} else {
			fmt.Println("❌ [AuthMiddleware] Invalid token claims")
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "Unauthorized",
				"message": "Invalid token claims",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}