package service

import (
	"context"
	"io"
)

// AssetUploader abstracts storage backends used for events, testimonials, and generic uploads.
type AssetUploader interface {
	Upload(ctx context.Context, objectKey string, contentType string, r io.Reader) (string, error)
	BuildEventAssetKey(eventID, kind, ext string) (string, error)
	BuildTestimonialImageKey(ext string) (string, error)
	BuildGenericAssetKey(folder, ext string) (string, error)
	BuildImageVariantKey(folder, assetID, variant, ext string) (string, error)
	NewAssetID() string
}
