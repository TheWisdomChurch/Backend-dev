package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func resetMemoryRateLimiters() {
	memMu.Lock()
	defer memMu.Unlock()
	memVisitors = make(map[string]*visitor)
}

func rateLimitedRouter(prefix string) *gin.Engine {
	router := gin.New()
	router.Use(RateLimiter(RateLimiterOptions{
		RequestsPerMinute: 1,
		Burst:             1,
		Window:            time.Minute,
		Prefix:            prefix,
	}))
	router.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	return router
}

func performRateLimitedRequest(router http.Handler) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/test", nil)
	request.RemoteAddr = "203.0.113.10:4321"
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestMemoryRateLimiterIsolatesPoliciesByPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetMemoryRateLimiters()

	global := rateLimitedRouter("rl:global")
	sermons := rateLimitedRouter("rl:sermons")

	if response := performRateLimitedRequest(global); response.Code != http.StatusNoContent {
		t.Fatalf("global limiter returned %d, want %d", response.Code, http.StatusNoContent)
	}
	if response := performRateLimitedRequest(sermons); response.Code != http.StatusNoContent {
		t.Fatalf("sermon limiter returned %d, want independent %d", response.Code, http.StatusNoContent)
	}
}

func TestMemoryRateLimiterStillLimitsWithinOnePolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetMemoryRateLimiters()

	router := rateLimitedRouter("rl:global")
	if response := performRateLimitedRequest(router); response.Code != http.StatusNoContent {
		t.Fatalf("first request returned %d, want %d", response.Code, http.StatusNoContent)
	}
	if response := performRateLimitedRequest(router); response.Code != http.StatusTooManyRequests {
		t.Fatalf("second request returned %d, want %d", response.Code, http.StatusTooManyRequests)
	}
}

func TestMemoryRateLimiterSkipsOnlyExactPublicReadPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetMemoryRateLimiters()

	router := gin.New()
	router.Use(RateLimiter(RateLimiterOptions{
		RequestsPerMinute: 1,
		Burst:             1,
		Window:            time.Minute,
		Prefix:            "rl:exact-skip",
		SkipPaths:         []string{"/api/v1/leadership"},
	}))
	router.GET("/api/v1/leadership", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.POST("/api/v1/leadership/apply", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	for range 2 {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/leadership", nil)
		request.RemoteAddr = "203.0.113.20:4321"
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("skipped read returned %d, want %d", response.Code, http.StatusNoContent)
		}
	}

	for attempt, want := range []int{http.StatusNoContent, http.StatusTooManyRequests} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/leadership/apply", nil)
		request.RemoteAddr = "203.0.113.20:4321"
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != want {
			t.Fatalf("apply attempt %d returned %d, want %d", attempt+1, response.Code, want)
		}
	}
}
