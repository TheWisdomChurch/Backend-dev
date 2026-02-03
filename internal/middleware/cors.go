package middleware

import (
	"log"
	"net/url"
	"regexp"
	"strings"
	"time"

	"wisdomHouse-backend/internal/config"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func CORS(cfg *config.CORSConfig) gin.HandlerFunc {
	allowedOrigins := cleanStringSlice(cfg.AllowedOrigins)
	allowedMethods := upperAll(cleanStringSlice(cfg.AllowedMethods))
	allowedHeaders := cleanStringSlice(cfg.AllowedHeaders)
	exposedHeaders := cleanStringSlice(cfg.ExposedHeaders)
	allowCredentials := cfg.AllowCredentials

	// ---- Defaults (safe) ----
	if len(allowedMethods) == 0 {
		allowedMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	}
	if len(allowedHeaders) == 0 {
		allowedHeaders = []string{
			"Origin",
			"Content-Type",
			"Authorization",
			"Accept",
			"X-Requested-With",
		}
	}

	// If credentials are allowed, "*" is invalid. Drop it.
	if allowCredentials {
		allowedOrigins = filterOutWildcard(allowedOrigins)
	}

	originValidator, exactOrigins := createOriginValidator(allowedOrigins, allowCredentials)

	corsConfig := cors.Config{
		AllowOrigins:     exactOrigins,    // exact matches only
		AllowMethods:     allowedMethods,
		AllowHeaders:     allowedHeaders,
		ExposeHeaders:    exposedHeaders,
		AllowCredentials: allowCredentials,
		MaxAge:           12 * time.Hour,

		// Wildcards + extra validation
		AllowOriginFunc: originValidator,
	}

	// Honor cfg.MaxAge if set
	if cfg.MaxAge > 0 {
		corsConfig.MaxAge = time.Duration(cfg.MaxAge) * time.Second
	}

	return cors.New(corsConfig)
}

func cleanStringSlice(slice []string) []string {
	cleaned := make([]string, 0, len(slice))
	for _, item := range slice {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}

func upperAll(slice []string) []string {
	out := make([]string, len(slice))
	for i, v := range slice {
		out[i] = strings.ToUpper(strings.TrimSpace(v))
	}
	return out
}

func filterOutWildcard(origins []string) []string {
	out := make([]string, 0, len(origins))
	for _, o := range origins {
		o = strings.TrimSpace(o)
		if o == "*" {
			continue
		}
		out = append(out, o)
	}
	return out
}

func normalizeOrigin(origin string) string {
	return strings.TrimSpace(origin)
}

// createOriginValidator returns:
// 1) validator func(origin) bool
// 2) exact origins slice for cors.Config.AllowOrigins
func createOriginValidator(allowedOrigins []string, allowCredentials bool) (func(string) bool, []string) {
	type wildcard struct {
		raw     string
		pattern *regexp.Regexp
	}

	wildcards := make([]wildcard, 0)
	exact := make(map[string]struct{}, len(allowedOrigins))
	exactList := make([]string, 0, len(allowedOrigins))

	allowAll := false

	for _, o := range allowedOrigins {
		o = normalizeOrigin(o)
		if o == "" {
			continue
		}

		// Allow all only when credentials are NOT used
		if o == "*" {
			if !allowCredentials {
				allowAll = true
			}
			continue
		}

		// wildcard entry like https://*.example.com
		if strings.Contains(o, "*") {
			escaped := regexp.QuoteMeta(o)
			escaped = strings.ReplaceAll(escaped, "\\*", ".*")
			re := regexp.MustCompile("^" + escaped + "$")
			wildcards = append(wildcards, wildcard{raw: o, pattern: re})
			continue
		}

		// exact origin
		if _, exists := exact[o]; !exists {
			exact[o] = struct{}{}
			exactList = append(exactList, o)
		}
	}

	return func(origin string) bool {
		origin = normalizeOrigin(origin)
		if origin == "" {
			// No Origin header should not be rejected by CORS; gin-cors calls validator only when Origin exists
			log.Printf("CORS: empty origin rejected")
			return false
		}

		// Must be a valid absolute origin: scheme + host
		u, err := url.Parse(origin)
		if err != nil || u.Scheme == "" || u.Host == "" {
			log.Printf("CORS: invalid origin url '%s': %v", origin, err)
			return false
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			log.Printf("CORS: invalid origin scheme '%s'", origin)
			return false
		}

		if allowAll {
			return true
		}

		if _, ok := exact[origin]; ok {
			return true
		}

		for _, w := range wildcards {
			if w.pattern.MatchString(origin) {
				return true
			}
		}

		log.Printf("CORS: rejected origin '%s'. allowed=%v allowCredentials=%t", origin, allowedOrigins, allowCredentials)
		return false
	}, exactList
}
