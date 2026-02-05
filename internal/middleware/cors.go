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

	if len(allowedMethods) == 0 {
		allowedMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	}
	if len(allowedHeaders) == 0 {
		allowedHeaders = []string{"Origin", "Content-Type", "Authorization", "Accept", "X-Requested-With"}
	}

	if allowCredentials {
		allowedOrigins = filterOutWildcard(allowedOrigins)
	}

	originValidator, exactOrigins := createOriginValidator(allowedOrigins, allowCredentials)

	corsConfig := cors.Config{
		AllowOrigins:     exactOrigins,
		AllowMethods:     allowedMethods,
		AllowHeaders:     allowedHeaders,
		ExposeHeaders:    exposedHeaders,
		AllowCredentials: allowCredentials,
		MaxAge:           12 * time.Hour,
		AllowOriginFunc:  originValidator,
	}

	if cfg.MaxAge > 0 {
		corsConfig.MaxAge = time.Duration(cfg.MaxAge) * time.Second
	}

	return cors.New(corsConfig)
}

func cleanStringSlice(slice []string) []string {
	out := make([]string, 0, len(slice))
	for _, v := range slice {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func upperAll(slice []string) []string {
	out := make([]string, 0, len(slice))
	for _, v := range slice {
		v = strings.ToUpper(strings.TrimSpace(v))
		if v != "" {
			out = append(out, v)
		}
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

func normalizeOrigin(origin string) string { return strings.TrimSpace(origin) }

func createOriginValidator(allowedOrigins []string, allowCredentials bool) (func(string) bool, []string) {
	type wildcard struct {
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
			if !allowCredentials {
				allowAll = true
			}
			continue
		}

		if strings.Contains(o, "*") {
			escaped := regexp.QuoteMeta(o)
			escaped = strings.ReplaceAll(escaped, "\\*", ".*")
			re := regexp.MustCompile("^" + escaped + "$")
			wildcards = append(wildcards, wildcard{pattern: re})
			continue
		}

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

		u, err := url.Parse(origin)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return false
		}
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
