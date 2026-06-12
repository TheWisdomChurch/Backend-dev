package authutil

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"wisdomHouse-backend/internal/cache"
)

const (
	geoIPCacheTTL        = 24 * time.Hour
	geoUserHistoryTTL    = 90 * 24 * time.Hour
	geoMaxKnownCountries = 5
)

// GeoLocation holds the country information returned by the ip-api.com range lookup.
type GeoLocation struct {
	Country     string `json:"country"`
	CountryCode string `json:"countryCode"`
	City        string `json:"city"`
}

// GeoDetector looks up IP geolocation and tracks per-user country history to
// detect logins from unusual locations. All operations fail open — a Redis or
// network error never blocks authentication.
type GeoDetector struct {
	httpClient *http.Client
	redis      *cache.RedisClient
}

// NewGeoDetector creates a GeoDetector backed by the given Redis client for caching.
func NewGeoDetector(r *cache.RedisClient) *GeoDetector {
	return &GeoDetector{
		httpClient: &http.Client{Timeout: 3 * time.Second},
		redis:      r,
	}
}

// LookupIP returns the geo location for the given IP, with a 24-hour Redis cache.
// Returns nil on loopback addresses or any error.
func (g *GeoDetector) LookupIP(ctx context.Context, ip string) *GeoLocation {
	if g == nil || strings.TrimSpace(ip) == "" {
		return nil
	}
	// Skip private/loopback addresses — they would return bogus data.
	if ip == "127.0.0.1" || ip == "::1" || strings.HasPrefix(ip, "192.168.") ||
		strings.HasPrefix(ip, "10.") || strings.HasPrefix(ip, "172.") {
		return nil
	}

	cacheKey := "geo:" + ip

	if g.redis != nil {
		if cached, err := g.redis.Get(ctx, cacheKey); err == nil {
			var loc GeoLocation
			if json.Unmarshal([]byte(cached), &loc) == nil {
				return &loc
			}
		}
	}

	resp, err := g.httpClient.Get("http://ip-api.com/json/" + ip + "?fields=country,countryCode,city")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var loc GeoLocation
	if err := json.NewDecoder(resp.Body).Decode(&loc); err != nil {
		return nil
	}
	if loc.CountryCode == "" {
		return nil
	}

	if g.redis != nil {
		if b, err := json.Marshal(loc); err == nil {
			_ = g.redis.Set(ctx, cacheKey, string(b), geoIPCacheTTL)
		}
	}

	return &loc
}

// IsNewCountry returns true when countryCode has not been seen in the user's recent
// login history. It also appends the country to the history (capped at
// geoMaxKnownCountries). Always returns false when Redis is unavailable.
func (g *GeoDetector) IsNewCountry(ctx context.Context, userID, countryCode string) bool {
	if g == nil || g.redis == nil || userID == "" || countryCode == "" {
		return false
	}

	key := "user:known_countries:" + userID

	var known []string
	if cached, err := g.redis.Get(ctx, key); err == nil {
		_ = json.Unmarshal([]byte(cached), &known)
	}

	for _, c := range known {
		if c == countryCode {
			return false
		}
	}

	// New country — append to the rolling history window.
	known = append(known, countryCode)
	if len(known) > geoMaxKnownCountries {
		known = known[len(known)-geoMaxKnownCountries:]
	}
	if b, err := json.Marshal(known); err == nil {
		_ = g.redis.Set(ctx, key, string(b), geoUserHistoryTTL)
	}

	return true
}
