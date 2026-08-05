package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNavigationServicePreviewRoute(t *testing.T) {
	t.Helper()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Goog-Api-Key"); got != "server-secret" {
			t.Fatalf("unexpected provider key: %q", got)
		}
		if got := r.Header.Get("X-Goog-FieldMask"); got != googleRoutesFieldMask {
			t.Fatalf("unexpected field mask: %q", got)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		destination := payload["destination"].(map[string]any)
		if destination["placeId"] != "verified-place" {
			t.Fatalf("route did not use verified place ID")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"routes":[{"distanceMeters":18400,"duration":"2100s","localizedValues":{"distance":{"text":"18.4 km"},"duration":{"text":"35 min"}},"polyline":{"encodedPolyline":"route-polyline"}}]}`))
	}))
	defer provider.Close()

	svc := NewNavigationService("server-secret", "verified-place", time.Second).(*navigationService)
	svc.endpoint = provider.URL

	preview, err := svc.PreviewRoute(t.Context(), Coordinates{Latitude: 6.4698, Longitude: 3.5852})
	if err != nil {
		t.Fatalf("PreviewRoute returned error: %v", err)
	}
	if preview.DurationSeconds != 2100 || preview.DistanceMeters != 18400 {
		t.Fatalf("unexpected preview: %#v", preview)
	}
	if preview.EncodedPolyline != "route-polyline" {
		t.Fatalf("unexpected polyline: %q", preview.EncodedPolyline)
	}
}

func TestNavigationServiceRejectsIncompleteProviderRoute(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"routes":[{"distanceMeters":100,"duration":"10s"}]}`))
	}))
	defer provider.Close()

	svc := NewNavigationService("server-secret", "verified-place", time.Second).(*navigationService)
	svc.endpoint = provider.URL
	if _, err := svc.PreviewRoute(t.Context(), Coordinates{}); err == nil {
		t.Fatal("expected incomplete route to be rejected")
	}
}
