// internal/middleware/logger.go
package middleware

import (
	"fmt"
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

// Logger accepts log level and returns a gin middleware
func Logger(logLevel string) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		timestamp := time.Now()
		latency := timestamp.Sub(start)

		if raw != "" {
			path = path + "?" + raw
		}

		// Color codes for terminal
		var statusColor string
		statusCode := c.Writer.Status()
		
		switch {
		case statusCode >= 500:
			statusColor = "\033[31m" // Red
		case statusCode >= 400:
			statusColor = "\033[33m" // Yellow
		case statusCode >= 300:
			statusColor = "\033[36m" // Cyan
		case statusCode >= 200:
			statusColor = "\033[32m" // Green
		default:
			statusColor = "\033[37m" // White
		}

		// Get client IP
		clientIP := c.ClientIP()
		if clientIP == "" {
			clientIP = "unknown"
		}

		// Format log message
		logMessage := ""
		
		switch logLevel {
		case "debug":
			logMessage = formatDebugLog(c, statusColor, path, clientIP, statusCode, latency)
		case "info", "warn", "error":
			logMessage = formatInfoLog(c, statusColor, path, clientIP, statusCode, latency)
		default:
			logMessage = formatInfoLog(c, statusColor, path, clientIP, statusCode, latency)
		}

		// Output log
		if logLevel == "error" && statusCode >= 400 {
			log.Printf("🔴 %s", logMessage)
		} else if logLevel == "warn" && statusCode >= 400 {
			log.Printf("🟡 %s", logMessage)
		} else {
			log.Printf("🟢 %s", logMessage)
		}

		// Log errors if any
		if len(c.Errors) > 0 {
			for _, e := range c.Errors.Errors() {
				log.Printf("❌ Error: %s", e)
			}
		}
	}
}

// formatDebugLog creates detailed debug log
func formatDebugLog(c *gin.Context, statusColor, path, clientIP string, statusCode int, latency time.Duration) string {
	userAgent := c.Request.UserAgent()
	if userAgent == "" {
		userAgent = "unknown"
	}
	
	return string(statusColor) + fmt.Sprintf("[%s] %s %s %d %v | User-Agent: %s",
		c.Request.Method,
		path,
		clientIP,
		statusCode,
		latency,
		userAgent,
	) + "\033[0m"
}

// formatInfoLog creates standard info log
func formatInfoLog(c *gin.Context, statusColor, path, clientIP string, statusCode int, latency time.Duration) string {
	return string(statusColor) + fmt.Sprintf("[%s] %s %s %d %v",
		c.Request.Method,
		path,
		clientIP,
		statusCode,
		latency,
	) + "\033[0m"
}

// SimpleLogger is your original simple logger (for backward compatibility)
func SimpleLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		timestamp := time.Now()
		latency := timestamp.Sub(start)

		if raw != "" {
			path = path + "?" + raw
		}

		log.Printf("[%s] %s %s %d %v",
			c.Request.Method,
			path,
			c.ClientIP(),
			c.Writer.Status(),
			latency,
		)
	}
}