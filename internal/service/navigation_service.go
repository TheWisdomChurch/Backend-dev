package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"wisdomHouse-backend/internal/apperror"
)

const googleRoutesEndpoint = "https://routes.googleapis.com/directions/v2:computeRoutes"
const googleRoutesFieldMask = "routes.distanceMeters,routes.duration,routes.localizedValues.distance.text,routes.localizedValues.duration.text,routes.polyline.encodedPolyline"

type Coordinates struct {
	Latitude  float64 `json:"latitude" binding:"gte=-90,lte=90"`
	Longitude float64 `json:"longitude" binding:"gte=-180,lte=180"`
}

type RoutePreview struct {
	DistanceMeters  int    `json:"distanceMeters"`
	DurationSeconds int    `json:"durationSeconds"`
	DistanceLabel   string `json:"distanceLabel"`
	DurationLabel   string `json:"durationLabel"`
	EncodedPolyline string `json:"encodedPolyline"`
}

type NavigationService interface {
	PreviewRoute(ctx context.Context, origin Coordinates) (*RoutePreview, error)
}

type navigationService struct {
	apiKey     string
	placeID    string
	endpoint   string
	httpClient *http.Client
}

func NewNavigationService(apiKey, placeID string, timeout time.Duration) NavigationService {
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	return &navigationService{
		apiKey:     strings.TrimSpace(apiKey),
		placeID:    strings.TrimSpace(placeID),
		endpoint:   googleRoutesEndpoint,
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (s *navigationService) PreviewRoute(ctx context.Context, origin Coordinates) (*RoutePreview, error) {
	if s.apiKey == "" || s.placeID == "" {
		return nil, apperror.ServiceUnavailable("navigation provider is not configured")
	}

	payload := map[string]any{
		"origin":                   map[string]any{"location": map[string]any{"latLng": origin}},
		"destination":              map[string]any{"placeId": s.placeID},
		"travelMode":               "DRIVE",
		"routingPreference":        "TRAFFIC_AWARE",
		"computeAlternativeRoutes": false,
		"languageCode":             "en",
		"units":                    "METRIC",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, apperror.Wrap(err, "encode navigation provider request")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, apperror.Wrap(err, "create navigation provider request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", s.apiKey)
	req.Header.Set("X-Goog-FieldMask", googleRoutesFieldMask)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, apperror.ServiceUnavailable("navigation provider request failed")
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, 1<<20)
	var providerResponse struct {
		Routes []struct {
			DistanceMeters  int    `json:"distanceMeters"`
			Duration        string `json:"duration"`
			LocalizedValues struct {
				Distance struct {
					Text string `json:"text"`
				} `json:"distance"`
				Duration struct {
					Text string `json:"text"`
				} `json:"duration"`
			} `json:"localizedValues"`
			Polyline struct {
				EncodedPolyline string `json:"encodedPolyline"`
			} `json:"polyline"`
		} `json:"routes"`
	}
	if err := json.NewDecoder(limited).Decode(&providerResponse); err != nil {
		return nil, apperror.ServiceUnavailable("navigation provider returned an invalid response")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || len(providerResponse.Routes) == 0 {
		return nil, apperror.ServiceUnavailable(fmt.Sprintf("navigation provider status %d", resp.StatusCode))
	}

	route := providerResponse.Routes[0]
	duration, err := time.ParseDuration(route.Duration)
	if err != nil || route.DistanceMeters <= 0 || route.Polyline.EncodedPolyline == "" {
		return nil, apperror.ServiceUnavailable("navigation provider returned an incomplete route")
	}
	return &RoutePreview{
		DistanceMeters:  route.DistanceMeters,
		DurationSeconds: int(duration.Round(time.Second).Seconds()),
		DistanceLabel:   route.LocalizedValues.Distance.Text,
		DurationLabel:   route.LocalizedValues.Duration.Text,
		EncodedPolyline: route.Polyline.EncodedPolyline,
	}, nil
}
