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

	httpClient *http.Client
}

func NewBunnyUploader(zone, key, region, pullZone, basePath string) *BunnyUploader {
	basePath = strings.Trim(basePath, "/")
	pullZone = strings.TrimRight(pullZone, "/")

	return &BunnyUploader{
		StorageZone:   zone,
		StorageKey:    key,
		StorageRegion: region,
		PullZone:      pullZone,
		BasePath:      basePath,
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // production-safe default
		},
	}
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

	uploadURL := fmt.Sprintf("https://%s.storage.bunnycdn.com/%s/%s", b.StorageRegion, b.StorageZone, objectKey)

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
