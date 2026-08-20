package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/realtime"
)

func TestSSEStreamDoesNotReflectUntrustedOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewSSEHandler(realtime.New(nil))
	router.GET("/stream", func(c *gin.Context) {
		c.Set("user_id", "admin-1")
		handler.Stream(c)
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/stream", nil).WithContext(ctx)
	req.Header.Set("Origin", "https://attacker.example")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("SSE handler reflected origin outside CORS middleware: %q", got)
	}
}
