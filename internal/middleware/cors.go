// internal/middleware/cors.go
package middleware

import (
	"regexp"
	"strings"
	"time"

	"wisdomHouse-backend/internal/config"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CORS returns a CORS middleware configured from application config
func CORS(cfg *config.CORSConfig) gin.HandlerFunc {
	// Clean and validate origins
	origins := cleanStringSlice(cfg.AllowedOrigins)
	
	// Clean and validate methods (convert to uppercase)
	methods := cleanStringSlice(cfg.AllowedMethods)
	for i, method := range methods {
		methods[i] = strings.ToUpper(method)
	}
	
	// Clean and validate headers
	headers := cleanStringSlice(cfg.AllowedHeaders)
	
	// Clean and validate exposed headers
	exposedHeaders := cleanStringSlice(cfg.ExposedHeaders)

	// Create CORS config
	corsConfig := cors.Config{
		AllowOrigins:     origins,
		AllowMethods:     methods,
		AllowHeaders:     headers,
		ExposeHeaders:    exposedHeaders,
		AllowCredentials: cfg.AllowCredentials,
		MaxAge:           time.Duration(cfg.MaxAge) * time.Second,
		AllowOriginFunc:  createOriginValidator(origins),
	}

	return cors.New(corsConfig)
}

// cleanStringSlice removes whitespace and empty strings from slice
func cleanStringSlice(slice []string) []string {
	cleaned := make([]string, 0, len(slice))
	for _, item := range slice {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}

// createOriginValidator creates a function to validate origins
func createOriginValidator(allowedOrigins []string) func(string) bool {
	return func(origin string) bool {
		// Allow all origins if specified
		for _, allowedOrigin := range allowedOrigins {
			if allowedOrigin == "*" {
				return true
			}
		}

		// Check exact match
		for _, allowedOrigin := range allowedOrigins {
			if origin == allowedOrigin {
				return true
			}
		}

		// Check pattern matching (e.g., *.example.com)
		for _, allowedOrigin := range allowedOrigins {
			if strings.Contains(allowedOrigin, "*") {
				pattern := strings.ReplaceAll(allowedOrigin, "*", ".*")
				if matched, _ := regexp.MatchString("^"+pattern+"$", origin); matched {
					return true
				}
			}
		}

		return false
	}
}

// CORSOptions returns CORS middleware with default options
func CORSOptions() gin.HandlerFunc {
	return cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	})
}