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

	// ---- Defaults (important for preflight stability) ----
	if len(allowedMethods) == 0 {
		allowedMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	}
	if len(allowedHeaders) == 0 {
		// Include what browsers commonly ask for in preflight
		allowedHeaders = []string{
			"Origin",
			"Content-Type",
			"Authorization",
			"Accept",
			"X-Requested-With",
		}
	}

	// IMPORTANT: if credentials are allowed, wildcard origins are invalid.
	if allowCredentials {
		allowedOrigins = filterOutWildcard(allowedOrigins)
	}

	// Build an origin validator supporting exact + wildcard.
	originValidator, exactOrigins := createOriginValidator(allowedOrigins, allowCredentials)

	corsConfig := cors.Config{
		// Setting AllowOrigins is important: gin-contrib/cors behaves more consistently
		// when this list contains exact origins (wildcards are handled via AllowOriginFunc).
		AllowOrigins:     exactOrigins,
		AllowMethods:     allowedMethods,
		AllowHeaders:     allowedHeaders,
		ExposeHeaders:    exposedHeaders,
		AllowCredentials: allowCredentials,

		// If MaxAge is 0 in cfg, set a sane default
		MaxAge: 12 * time.Hour,

		// Use function for wildcard matching + extra validation
		AllowOriginFunc: originValidator,
	}

	// If cfg.MaxAge is explicitly provided, honor it
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
		out[i] = strings.ToUpper(v)
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
// 1) a validator func(origin) bool
// 2) the list of exact origins (non-wildcard) for cors.Config.AllowOrigins
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

		if o == "*" {
			// Only permissible when credentials are NOT used.
			if !allowCredentials {
				allowAll = true
			}
			continue
		}

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
			return false
		}

		// Must be a valid absolute origin: scheme + host
		u, err := url.Parse(origin)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return false
		}
		// Only allow http/https origins
		if u.Scheme != "http" && u.Scheme != "https" {
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
	}, exactList
}
