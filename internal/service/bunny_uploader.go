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
)

// BunnyUploader handles uploads to BunnyCDN storage and returns the public Pull Zone URL.
type BunnyUploader struct {
	StorageZone   string
	StorageKey    string
	StorageRegion string // e.g. "de"
	PullZone      string // e.g. "https://wisdom-church.b-cdn.net"
	BasePath      string // e.g. "uploads"
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
func (b *BunnyUploader) Upload(ctx context.Context, objectKey string, contentType string, r io.Reader) (cdnURL string, err error) {
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

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("bunny upload failed: status=%d", resp.StatusCode)
	}

	cdnURL = b.PullZone + "/" + objectKey
	return cdnURL, nil
}

// BuildEventImageKey produces a randomized object key under the configured base path.
func (b *BunnyUploader) BuildEventImageKey(eventID, ext string) (string, error) {
	token, err := randHex(16)
	if err != nil {
		return "", err
	}

	fileName := token + "." + strings.TrimLeft(ext, ".")
	if b.BasePath == "" {
		return path.Join("events", eventID, fileName), nil
	}
	return path.Join(b.BasePath, "events", eventID, fileName), nil
}
