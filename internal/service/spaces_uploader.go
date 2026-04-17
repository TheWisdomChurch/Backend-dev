package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"
)

// SpacesUploader implements AssetUploader using S3-compatible object storage
// (Supabase S3, DigitalOcean Spaces, and similar providers).
type SpacesUploader struct {
	bucket        string
	basePath      string
	publicBaseURL string
	publicRead    bool
	provider      string
	client        *s3.Client
}

// NewSpacesUploaderFromEnv builds an uploader from S3_* env vars
// with SPACES_* kept as backward-compatible fallbacks.
// Returns (nil, nil) when no storage configuration is present.
func NewSpacesUploaderFromEnv() (*SpacesUploader, error) {
	bucket := firstEnv("S3_BUCKET", "SPACES_BUCKET")
	accessKey := firstEnv("S3_ACCESS_KEY", "SPACES_ACCESS_KEY")
	secretKey := firstEnv("S3_SECRET_KEY", "SPACES_SECRET_KEY")
	endpoint := firstEnv("S3_ENDPOINT", "SPACES_ENDPOINT")
	region := firstEnv("S3_REGION", "SPACES_REGION")
	publicBaseURL := strings.TrimRight(
		firstEnv("S3_PUBLIC_BASE_URL", "SPACES_PUBLIC_BASE_URL"),
		"/",
	)
	basePath := strings.Trim(firstEnv("S3_BASE_PATH", "SPACES_BASE_PATH"), "/")
	provider := strings.ToLower(firstEnv("S3_PROVIDER", "STORAGE_PROVIDER"))
	if provider == "" {
		provider = "s3"
	}

	if bucket == "" && accessKey == "" && secretKey == "" && endpoint == "" && publicBaseURL == "" && region == "" {
		return nil, nil
	}

	if bucket == "" || accessKey == "" || secretKey == "" {
		return nil, errors.New("storage config incomplete: require S3_BUCKET, S3_ACCESS_KEY, S3_SECRET_KEY")
	}

	if region == "" {
		region = "us-east-1"
	}

	endpoint = normalizeEndpoint(endpoint)
	if publicBaseURL == "" {
		publicBaseURL = derivePublicBaseURL(bucket, endpoint, region)
	}
	if publicBaseURL == "" {
		return nil, errors.New("S3_PUBLIC_BASE_URL is required to build public URLs")
	}

	publicRead := parseBoolEnvWithFallback([]string{"S3_PUBLIC_READ", "SPACES_PUBLIC_READ"}, true)

	loadOpts := []func(*config.LoadOptions) error{
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	}
	if endpoint != "" {
		resolver := aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				if service == s3.ServiceID {
					return aws.Endpoint{
						URL:               endpoint,
						HostnameImmutable: true,
					}, nil
				}
				return aws.Endpoint{}, &aws.EndpointNotFoundError{}
			},
		)
		loadOpts = append(loadOpts, config.WithEndpointResolverWithOptions(resolver))
	}

	cfg, err := config.LoadDefaultConfig(context.Background(), loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("spaces config load failed: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if endpoint != "" {
			o.UsePathStyle = true
		}
	})

	return &SpacesUploader{
		bucket:        bucket,
		basePath:      basePath,
		publicBaseURL: publicBaseURL,
		publicRead:    publicRead,
		provider:      provider,
		client:        client,
	}, nil
}

func (s *SpacesUploader) Upload(ctx context.Context, objectKey string, contentType string, r io.Reader) (string, error) {
	if s == nil || s.client == nil {
		return "", errors.New("uploader not configured")
	}
	key := strings.TrimLeft(strings.TrimSpace(objectKey), "/")
	if key == "" {
		return "", errors.New("object key is required")
	}

	input := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   r,
	}
	if strings.TrimSpace(contentType) != "" {
		input.ContentType = aws.String(contentType)
	}
	if s.publicRead {
		input.ACL = types.ObjectCannedACLPublicRead
	}

	if _, err := s.client.PutObject(ctx, input); err != nil {
		return "", err
	}

	return strings.TrimRight(s.publicBaseURL, "/") + "/" + key, nil
}

// PresignPut creates a pre-signed PUT URL for direct uploads.
func (s *SpacesUploader) PresignPut(ctx context.Context, objectKey string, contentType string, expires time.Duration) (string, error) {
	if s == nil || s.client == nil {
		return "", errors.New("uploader not configured")
	}
	key := strings.TrimLeft(strings.TrimSpace(objectKey), "/")
	if key == "" {
		return "", errors.New("object key is required")
	}
	if expires <= 0 {
		expires = 15 * time.Minute
	}

	input := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}
	if strings.TrimSpace(contentType) != "" {
		input.ContentType = aws.String(contentType)
	}
	if s.publicRead {
		input.ACL = types.ObjectCannedACLPublicRead
	}

	presigner := s3.NewPresignClient(s.client)
	out, err := presigner.PresignPutObject(ctx, input, s3.WithPresignExpires(expires))
	if err != nil {
		return "", err
	}
	return out.URL, nil
}

func (s *SpacesUploader) BuildEventAssetKey(eventID, kind, ext string) (string, error) {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return "", errors.New("eventID is required")
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		return "", errors.New("kind is required")
	}
	ext = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".")
	if ext == "" {
		return "", errors.New("ext is required")
	}

	key := path.Join("events", eventID, fmt.Sprintf("%s.%s", kind, ext))
	return s.withBasePath(key), nil
}

func (s *SpacesUploader) BuildTestimonialImageKey(ext string) (string, error) {
	ext = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".")
	if ext == "" {
		return "", errors.New("ext is required")
	}
	key := path.Join("testimonials", uuid.NewString()+"."+ext)
	return s.withBasePath(key), nil
}

func (s *SpacesUploader) BuildGenericAssetKey(folder, ext string) (string, error) {
	ext = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".")
	if ext == "" {
		return "", errors.New("ext is required")
	}
	folder, err := sanitizeFolder(folder)
	if err != nil {
		return "", err
	}
	key := path.Join(folder, uuid.NewString()+"."+ext)
	return s.withBasePath(key), nil
}

func (s *SpacesUploader) withBasePath(key string) string {
	if s == nil {
		return key
	}
	k := strings.TrimLeft(strings.TrimSpace(key), "/")
	if s.basePath == "" {
		return k
	}
	return path.Join(s.basePath, k)
}

func (s *SpacesUploader) PublicBaseURL() string {
	if s == nil {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(s.publicBaseURL), "/")
}

func (s *SpacesUploader) Bucket() string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(s.bucket)
}

func (s *SpacesUploader) ProviderName() string {
	if s == nil {
		return "s3"
	}
	provider := strings.TrimSpace(s.provider)
	if provider == "" {
		return "s3"
	}
	return provider
}

func sanitizeFolder(folder string) (string, error) {
	f := strings.Trim(strings.TrimSpace(folder), "/")
	if f == "" {
		return "uploads", nil
	}
	parts := strings.Split(f, "/")
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return "", errors.New("invalid folder")
		}
		if strings.Contains(p, "..") {
			return "", errors.New("invalid folder")
		}
	}
	return f, nil
}

func normalizeEndpoint(endpoint string) string {
	raw := strings.TrimSpace(endpoint)
	if raw == "" {
		return ""
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return raw
	}
	return "https://" + raw
}

func derivePublicBaseURL(bucket, endpoint, region string) string {
	if bucket == "" {
		return ""
	}

	if endpoint != "" {
		u, err := url.Parse(endpoint)
		if err == nil {
			scheme := u.Scheme
			host := u.Host
			if host == "" {
				host = strings.TrimSpace(u.Path)
			}
			if scheme == "" {
				scheme = "https"
			}
			if host != "" {
				if !strings.HasPrefix(host, bucket+".") {
					host = bucket + "." + host
				}
				return scheme + "://" + host
			}
		}
	}

	if strings.TrimSpace(region) != "" {
		return fmt.Sprintf("https://%s.%s.digitaloceanspaces.com", bucket, strings.TrimSpace(region))
	}

	return ""
}

func parseBoolEnv(key string, defaultValue bool) bool {
	val := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if val == "" {
		return defaultValue
	}
	switch val {
	case "1", "true", "t", "yes", "y", "on":
		return true
	case "0", "false", "f", "no", "n", "off":
		return false
	default:
		return defaultValue
	}
}

func parseBoolEnvWithFallback(keys []string, defaultValue bool) bool {
	for _, key := range keys {
		val := strings.TrimSpace(os.Getenv(key))
		if val == "" {
			continue
		}
		switch strings.ToLower(val) {
		case "1", "true", "t", "yes", "y", "on":
			return true
		case "0", "false", "f", "no", "n", "off":
			return false
		default:
			return defaultValue
		}
	}
	return defaultValue
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		val := strings.TrimSpace(os.Getenv(key))
		if val != "" {
			return val
		}
	}
	return ""
}
