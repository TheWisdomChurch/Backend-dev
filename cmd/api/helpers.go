package main

import (
	"os"
	"strings"

	"wisdomHouse-backend/internal/config"
)

func isTrueEnv(key string) bool {
	val := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch val {
	case "1", "true", "t", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func hasAnyEnv(keys ...string) bool {
	for _, key := range keys {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return true
		}
	}
	return false
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

func ensureCORSDefaults(cfg *config.Config) {
	if cfg == nil {
		return
	}

	existing := make(map[string]struct{}, len(cfg.CORS.AllowedOrigins))
	for _, o := range cfg.CORS.AllowedOrigins {
		o = normalizeCORSOrigin(o)
		if o == "" {
			continue
		}
		existing[o] = struct{}{}
	}

	candidates := []string{
		normalizeCORSOrigin(cfg.App.FrontendURL),
		normalizeCORSOrigin(cfg.App.AdminPortalURL),
		"https://wisdomchurchhq.org",
		"https://www.wisdomchurchhq.org",
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if _, ok := existing[c]; ok {
			continue
		}
		cfg.CORS.AllowedOrigins = append(cfg.CORS.AllowedOrigins, c)
		existing[c] = struct{}{}
	}

	if len(cfg.CORS.AllowedOrigins) == 0 {
		cfg.CORS.AllowedOrigins = []string{"http://localhost:3000", "http://localhost:3001"}
	}
}

func normalizeCORSOrigin(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	if strings.Contains(value, "://") {
		return ""
	}
	return "https://" + value
}
