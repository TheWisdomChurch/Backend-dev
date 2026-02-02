package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"
)

// BunnyUploader handles uploads to BunnyCDN storage and returns the public Pull Zone URL.
type BunnyUploader struct {
	StorageZone   string
	StorageKey    string
	StorageRegion string
	PullZone      string
	BasePath      string

	storageEndpoint string
	httpClient      *http.Client
}

// BunnyOption customizes uploader construction (primarily for tests/mocks).
type BunnyOption func(*BunnyUploader)

// WithHTTPClient injects a custom HTTP client (e.g., to stub network).
func WithHTTPClient(c *http.Client) BunnyOption {
	return func(b *BunnyUploader) {
		b.httpClient = c
	}
}

// WithEndpoint overrides the storage endpoint (default: https://<region>.storage.bunnycdn.com).
func WithEndpoint(endpoint string) BunnyOption {
	endpoint = strings.TrimRight(endpoint, "/")
	return func(b *BunnyUploader) {
		b.storageEndpoint = endpoint
	}
}

func NewBunnyUploader(zone, key, region, pullZone, basePath string, opts ...BunnyOption) *BunnyUploader {
	basePath = strings.Trim(basePath, "/")
	pullZone = strings.TrimRight(pullZone, "/")
	region = strings.TrimSpace(region)

	uploader := &BunnyUploader{
		StorageZone:     zone,
		StorageKey:      key,
		StorageRegion:   region,
		PullZone:        pullZone,
		BasePath:        basePath,
		storageEndpoint: "",
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // production-safe default
		},
	}

	for _, opt := range opts {
		if opt != nil {
			opt(uploader)
		}
	}

	if uploader.storageEndpoint == "" {
		uploader.storageEndpoint = fmt.Sprintf("https://%s.storage.bunnycdn.com", uploader.StorageRegion)
	}

	return uploader
}

func randHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Upload streams the reader to Bunny Storage and returns the pull-zone URL.
func (b *BunnyUploader) Upload(ctx context.Context, objectKey string, contentType string, r io.Reader) (string, error) {
	objectKey = strings.TrimLeft(objectKey, "/")
	uploadURL := fmt.Sprintf("%s/%s/%s", strings.TrimRight(b.storageEndpoint, "/"), b.StorageZone, objectKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, r)
	if err != nil {
		return "", err
	}

	req.Header.Set("AccessKey", b.StorageKey)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("bunny upload failed: status=%d", resp.StatusCode)
	}

	return b.PullZone + "/" + objectKey, nil
}

// BuildEventAssetKey produces a randomized object key under base path, separated by kind (image/banner).
func (b *BunnyUploader) BuildEventAssetKey(eventID, kind, ext string) (string, error) {
	token, err := randHex(16)
	if err != nil {
		return "", err
	}

	ext = strings.TrimLeft(ext, ".")
	fileName := token + "." + ext

	if b.BasePath == "" {
		return path.Join("events", eventID, kind, fileName), nil
	}
	return path.Join(b.BasePath, "events", eventID, kind, fileName), nil
}

// BuildTestimonialImageKey produces a randomized object key for testimonial images.
func (b *BunnyUploader) BuildTestimonialImageKey(ext string) (string, error) {
	token, err := randHex(16)
	if err != nil {
		return "", err
	}

	ext = strings.TrimLeft(ext, ".")
	fileName := token + "." + ext

	if b.BasePath == "" {
		return path.Join("testimonials", fileName), nil
	}
	return path.Join(b.BasePath, "testimonials", fileName), nil
}

// BuildGenericAssetKey builds a randomized object key for a generic folder (e.g., admin uploads).
// Folder is trimmed of slashes; defaults to "uploads" if empty.
func (b *BunnyUploader) BuildGenericAssetKey(folder, ext string) (string, error) {
	token, err := randHex(16)
	if err != nil {
		return "", err
	}

	folder = strings.Trim(folder, "/")
	if folder == "" {
		folder = "uploads"
	}

	ext = strings.TrimLeft(ext, ".")
	fileName := token + "." + ext

	if b.BasePath == "" {
		return path.Join(folder, fileName), nil
	}
	return path.Join(b.BasePath, folder, fileName), nil
}
