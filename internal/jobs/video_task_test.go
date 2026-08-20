package jobs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestDownloadToTempRejectsOversizedContentLength(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "536870913")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	handler := &VideoProcessHandler{}
	path, err := handler.downloadToTemp(context.Background(), server.URL)
	if err == nil || !strings.Contains(err.Error(), "processing limit") {
		t.Fatalf("expected processing-limit error, got path=%q err=%v", path, err)
	}
	if path != "" {
		_ = os.Remove(path)
		t.Fatalf("oversized response unexpectedly created %q", path)
	}
}

func TestDownloadToTempRejectsNonSuccessResponse(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	path, err := (&VideoProcessHandler{}).downloadToTemp(context.Background(), server.URL)
	if err == nil || !strings.Contains(err.Error(), "unexpected status") {
		t.Fatalf("expected status error, got path=%q err=%v", path, err)
	}
}
