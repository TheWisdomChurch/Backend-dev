package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUploadUsesCustomEndpoint(t *testing.T) {
	var gotPath string
	var gotAccess string
	var gotBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAccess = r.Header.Get("AccessKey")
		data, _ := io.ReadAll(r.Body)
		gotBody = string(data)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	uploader := NewBunnyUploader(
		"zone123",
		"secret-key",
		"ny",
		"https://pull.example",
		"uploads",
		WithEndpoint(srv.URL),
	)

	ctx := context.Background()
	url, err := uploader.Upload(ctx, "events/evt-1/image/foo.png", "image/png", strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("upload returned error: %v", err)
	}

	if url != "https://pull.example/events/evt-1/image/foo.png" {
		t.Fatalf("unexpected CDN url: %s", url)
	}
	if gotPath != "/zone123/events/evt-1/image/foo.png" {
		t.Fatalf("unexpected storage path: %s", gotPath)
	}
	if gotAccess != "secret-key" {
		t.Fatalf("missing AccessKey header: %s", gotAccess)
	}
	if gotBody != "hello" {
		t.Fatalf("unexpected body: %s", gotBody)
	}
}

func TestBuildEventAssetKey(t *testing.T) {
	uploader := NewBunnyUploader("zone", "key", "de", "https://pull", "media")
	key, err := uploader.BuildEventAssetKey("evt-9", "banner", "jpg")
	if err != nil {
		t.Fatalf("BuildEventAssetKey error: %v", err)
	}

	parts := strings.Split(key, "/")
	if len(parts) != 5 {
		t.Fatalf("expected 5 path parts, got %d: %v", len(parts), parts)
	}
	if parts[0] != "media" || parts[1] != "events" || parts[2] != "evt-9" || parts[3] != "banner" {
		t.Fatalf("unexpected prefix: %v", parts[:4])
	}
	if !strings.HasSuffix(parts[4], ".jpg") {
		t.Fatalf("filename should end with .jpg: %s", parts[4])
	}
}

func TestBuildTestimonialImageKey(t *testing.T) {
	uploader := NewBunnyUploader("zone", "key", "de", "https://pull", "")
	key, err := uploader.BuildTestimonialImageKey("png")
	if err != nil {
		t.Fatalf("BuildTestimonialImageKey error: %v", err)
	}

	parts := strings.Split(key, "/")
	if len(parts) != 2 || parts[0] != "testimonials" {
		t.Fatalf("unexpected path: %v", parts)
	}
	if !strings.HasSuffix(parts[1], ".png") {
		t.Fatalf("filename should end with .png: %s", parts[1])
	}
}
