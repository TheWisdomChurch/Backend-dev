package service

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// SpacesUploader uploads assets to DigitalOcean Spaces (S3-compatible) and returns public URLs.
type SpacesUploader struct {
	Bucket        string
	Region        string
	Endpoint      string
	PublicBaseURL string
	BasePath      string
	PublicRead    bool

	client *s3.Client
}

// NewSpacesUploader creates a new uploader for DigitalOcean Spaces.
func NewSpacesUploader(bucket, region, endpoint, accessKey, secretKey, publicBaseURL, basePath string, publicRead bool) (*SpacesUploader, error) {
	bucket = strings.TrimSpace(bucket)
	region = strings.TrimSpace(region)
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	basePath = strings.Trim(basePath, "/")

	if bucket == "" {
		return nil, fmt.Errorf("spaces bucket is required")
	}
	if region == "" {
		return nil, fmt.Errorf("spaces region is required")
	}
	if strings.TrimSpace(accessKey) == "" || strings.TrimSpace(secretKey) == "" {
		return nil, fmt.Errorf("spaces access key and secret key are required")
	}

	if endpoint == "" {
		endpoint = fmt.Sprintf("https://%s.digitaloceanspaces.com", region)
	}
	if publicBaseURL == "" {
		publicBaseURL = fmt.Sprintf("https://%s.%s.digitaloceanspaces.com", bucket, region)
	}

	awsCfg, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		config.WithEndpointResolverWithOptions(
			aws.EndpointResolverWithOptionsFunc(func(service, region string, _ ...interface{}) (aws.Endpoint, error) {
				if service == s3.ServiceID {
					return aws.Endpoint{
						URL:               endpoint,
						HostnameImmutable: true,
					}, nil
				}
				return aws.Endpoint{}, &aws.EndpointNotFoundError{}
			}),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("spaces config load failed: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})

	return &SpacesUploader{
		Bucket:        bucket,
		Region:        region,
		Endpoint:      endpoint,
		PublicBaseURL: publicBaseURL,
		BasePath:      basePath,
		PublicRead:    publicRead,
		client:        client,
	}, nil
}

// Upload streams the reader to Spaces and returns the public URL.
func (s *SpacesUploader) Upload(ctx context.Context, objectKey string, contentType string, r io.Reader) (string, error) {
	objectKey = strings.TrimLeft(objectKey, "/")

	input := &s3.PutObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(objectKey),
		Body:   r,
	}
	if strings.TrimSpace(contentType) != "" {
		input.ContentType = aws.String(contentType)
	}
	if s.PublicRead {
		input.ACL = types.ObjectCannedACLPublicRead
	}

	if _, err := s.client.PutObject(ctx, input); err != nil {
		return "", fmt.Errorf("spaces upload failed: %w", err)
	}

	base := strings.TrimRight(s.PublicBaseURL, "/")
	if base == "" {
		return "", fmt.Errorf("spaces public base url is required")
	}

	return base + "/" + objectKey, nil
}

// BuildEventAssetKey produces a randomized object key under base path, separated by kind (image/banner).
func (s *SpacesUploader) BuildEventAssetKey(eventID, kind, ext string) (string, error) {
	token, err := randHex(16)
	if err != nil {
		return "", err
	}

	ext = strings.TrimLeft(ext, ".")
	fileName := token + "." + ext

	if s.BasePath == "" {
		return path.Join("events", eventID, kind, fileName), nil
	}
	return path.Join(s.BasePath, "events", eventID, kind, fileName), nil
}

// BuildTestimonialImageKey produces a randomized object key for testimonial images.
func (s *SpacesUploader) BuildTestimonialImageKey(ext string) (string, error) {
	token, err := randHex(16)
	if err != nil {
		return "", err
	}

	ext = strings.TrimLeft(ext, ".")
	fileName := token + "." + ext

	if s.BasePath == "" {
		return path.Join("testimonials", fileName), nil
	}
	return path.Join(s.BasePath, "testimonials", fileName), nil
}

// BuildGenericAssetKey builds a randomized object key for a generic folder (e.g., admin uploads).
// Folder is trimmed of slashes; defaults to "uploads" if empty.
func (s *SpacesUploader) BuildGenericAssetKey(folder, ext string) (string, error) {
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

	if s.BasePath == "" {
		return path.Join(folder, fileName), nil
	}
	return path.Join(s.BasePath, folder, fileName), nil
}

// URLForKey builds a public URL for an existing object key.
func (s *SpacesUploader) URLForKey(objectKey string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(s.PublicBaseURL), "/")
	if base == "" {
		return "", fmt.Errorf("spaces public base url is required")
	}
	key := strings.TrimLeft(strings.TrimSpace(objectKey), "/")
	if key == "" {
		return "", fmt.Errorf("object key is required")
	}
	return base + "/" + key, nil
}

// ParseEndpointHost returns the host portion of the configured endpoint.
func (s *SpacesUploader) ParseEndpointHost() string {
	if strings.TrimSpace(s.Endpoint) == "" {
		return ""
	}
	u, err := url.Parse(s.Endpoint)
	if err != nil {
		return ""
	}
	return u.Host
}

// DefaultTimeout returns a reasonable upload timeout.
func (s *SpacesUploader) DefaultTimeout() time.Duration {
	return 60 * time.Second
}
