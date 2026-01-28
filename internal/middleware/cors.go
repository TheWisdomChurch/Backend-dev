package middleware

import (
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

	// IMPORTANT: if credentials are allowed, wildcard origins are invalid.
	if allowCredentials {
		allowedOrigins = filterOutWildcard(allowedOrigins)
	}

	originValidator := createOriginValidator(allowedOrigins, allowCredentials)

	corsConfig := cors.Config{
		AllowMethods:     allowedMethods,
		AllowHeaders:     allowedHeaders,
		ExposeHeaders:    exposedHeaders,
		AllowCredentials: allowCredentials,
		MaxAge:           time.Duration(cfg.MaxAge) * time.Second,

		// We validate origins with a function; do not rely on "*" in AllowOrigins.
		AllowOriginFunc: originValidator,
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
		out[i] = strings.ToUpper(v)
	}
	return out
}

func filterOutWildcard(origins []string) []string {
	out := make([]string, 0, len(origins))
	for _, o := range origins {
		if o == "*" {
			continue
		}
		out = append(out, o)
	}
	return out
}

func normalizeOrigin(origin string) string {
	// Browsers send origins like "https://example.com"
	origin = strings.TrimSpace(origin)
	return origin
}

func createOriginValidator(allowedOrigins []string, allowCredentials bool) func(string) bool {
	// Precompile wildcard patterns
	type wildcard struct {
		raw     string
		pattern *regexp.Regexp
	}
	wildcards := make([]wildcard, 0)

	exact := make(map[string]struct{}, len(allowedOrigins))
	allowAll := false

	for _, o := range allowedOrigins {
		o = normalizeOrigin(o)
		if o == "" {
			continue
		}

		if o == "*" {
			// Only permissible when credentials are NOT used.
			if !allowCredentials {
				allowAll = true
			}
			continue
		}

		if strings.Contains(o, "*") {
			// Convert e.g. https://*.example.com to regex
			escaped := regexp.QuoteMeta(o)
			escaped = strings.ReplaceAll(escaped, "\\*", ".*")
			re := regexp.MustCompile("^" + escaped + "$")
			wildcards = append(wildcards, wildcard{raw: o, pattern: re})
			continue
		}

		exact[o] = struct{}{}
	}

	return func(origin string) bool {
		origin = normalizeOrigin(origin)
		if origin == "" {
			return false
		}

		// Must be a valid origin URL
		if _, err := url.Parse(origin); err != nil {
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

		return false
	}
}
