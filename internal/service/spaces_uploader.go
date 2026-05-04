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

// S3Uploader implements AssetUploader using S3-compatible object storage.
type S3Uploader struct {
	bucket        string
	basePath      string
	publicBaseURL string
	publicRead    bool
	provider      string
	endpoint      string
	client        *s3.Client
}

// NewS3UploaderFromEnv builds an uploader from S3_* env vars.
// Returns (nil, nil) when no storage configuration is present.
func NewS3UploaderFromEnv() (*S3Uploader, error) {
	bucket := strings.TrimSpace(firstEnv("S3_BUCKET"))
	accessKey := strings.TrimSpace(firstEnv("S3_ACCESS_KEY"))
	secretKey := strings.TrimSpace(firstEnv("S3_SECRET_KEY"))
	endpoint := normalizeEndpoint(firstEnv("S3_ENDPOINT"))
	region := strings.TrimSpace(firstEnv("S3_REGION"))
	publicBaseURL := strings.TrimRight(strings.TrimSpace(firstEnv("S3_PUBLIC_BASE_URL")), "/")
	basePath := strings.Trim(strings.TrimSpace(firstEnv("S3_BASE_PATH")), "/")
	provider := strings.ToLower(strings.TrimSpace(firstEnv("S3_PROVIDER", "STORAGE_PROVIDER")))

	if provider == "" {
		provider = "s3"
	}

	if bucket == "" &&
		accessKey == "" &&
		secretKey == "" &&
		endpoint == "" &&
		publicBaseURL == "" &&
		region == "" {
		return nil, nil
	}

	if bucket == "" {
		return nil, errors.New("storage config incomplete: S3_BUCKET is required")
	}

	if accessKey == "" {
		return nil, errors.New("storage config incomplete: S3_ACCESS_KEY is required")
	}

	if secretKey == "" {
		return nil, errors.New("storage config incomplete: S3_SECRET_KEY is required")
	}

	if region == "" {
		region = "us-east-1"
	}

	if publicBaseURL == "" {
		publicBaseURL = derivePublicBaseURL(bucket, endpoint, region)
	}

	if publicBaseURL == "" {
		return nil, errors.New("S3_PUBLIC_BASE_URL is required to build public URLs")
	}

	// Supabase Storage S3 should not receive x-amz-acl: public-read.
	// Bucket visibility/policy controls public access.
	publicReadDefault := true
	if isSupabaseStorageS3Endpoint(endpoint) {
		publicReadDefault = false
	}

	publicRead := parseBoolEnv("S3_PUBLIC_READ", publicReadDefault)
	if shouldDisableObjectACL(endpoint, provider) {
		publicRead = false
	}

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
		return nil, fmt.Errorf("s3 config load failed: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if endpoint != "" {
			o.UsePathStyle = true
		}
	})

	return &S3Uploader{
		bucket:        bucket,
		basePath:      basePath,
		publicBaseURL: publicBaseURL,
		publicRead:    publicRead,
		provider:      provider,
		endpoint:      endpoint,
		client:        client,
	}, nil
}

func (s *S3Uploader) Upload(ctx context.Context, objectKey string, contentType string, r io.Reader) (string, error) {
	if s == nil || s.client == nil {
		return "", errors.New("uploader not configured")
	}

	if r == nil {
		return "", errors.New("upload body is required")
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

	if ct := strings.TrimSpace(contentType); ct != "" {
		input.ContentType = aws.String(ct)
	}

	if cacheControl := strings.TrimSpace(os.Getenv("S3_CACHE_CONTROL")); cacheControl != "" {
		input.CacheControl = aws.String(cacheControl)
	}

	if s.publicRead && !shouldDisableObjectACL(s.endpoint, s.provider) {
		input.ACL = types.ObjectCannedACLPublicRead
	}

	if _, err := s.client.PutObject(ctx, input); err != nil {
		return "", fmt.Errorf("s3 put object failed for bucket %q key %q: %w", s.bucket, key, err)
	}

	return joinPublicURL(s.publicBaseURL, key), nil
}

// PresignPut creates a pre-signed PUT URL for direct uploads.
func (s *S3Uploader) PresignPut(ctx context.Context, objectKey string, contentType string, expires time.Duration) (string, error) {
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

	if ct := strings.TrimSpace(contentType); ct != "" {
		input.ContentType = aws.String(ct)
	}

	if cacheControl := strings.TrimSpace(os.Getenv("S3_CACHE_CONTROL")); cacheControl != "" {
		input.CacheControl = aws.String(cacheControl)
	}

	if s.publicRead && !shouldDisableObjectACL(s.endpoint, s.provider) {
		input.ACL = types.ObjectCannedACLPublicRead
	}

	presigner := s3.NewPresignClient(s.client)

	out, err := presigner.PresignPutObject(ctx, input, s3.WithPresignExpires(expires))
	if err != nil {
		return "", fmt.Errorf("s3 presign put object failed for bucket %q key %q: %w", s.bucket, key, err)
	}

	return out.URL, nil
}

func (s *S3Uploader) BuildEventAssetKey(eventID, kind, ext string) (string, error) {
	eventID = s3SanitizeSegment(eventID)
	if eventID == "" || eventID == "unknown" {
		return "", errors.New("eventID is required")
	}

	kind = s3SanitizeSegment(kind)
	if kind == "" || kind == "unknown" {
		return "", errors.New("kind is required")
	}

	ext = s3SanitizeExtension(ext)
	if ext == "" {
		return "", errors.New("ext is required")
	}

	key := path.Join("events", eventID, fmt.Sprintf("%s.%s", kind, ext))
	return s.withBasePath(key), nil
}

func (s *S3Uploader) BuildTestimonialImageKey(ext string) (string, error) {
	ext = s3SanitizeExtension(ext)
	if ext == "" {
		return "", errors.New("ext is required")
	}

	key := path.Join("testimonials", uuid.NewString()+"."+ext)
	return s.withBasePath(key), nil
}

func (s *S3Uploader) BuildGenericAssetKey(folder, ext string) (string, error) {
	ext = s3SanitizeExtension(ext)
	if ext == "" {
		return "", errors.New("ext is required")
	}

	cleanFolder, err := sanitizeFolder(folder)
	if err != nil {
		return "", err
	}

	key := path.Join(cleanFolder, uuid.NewString()+"."+ext)
	return s.withBasePath(key), nil
}

func (s *S3Uploader) withBasePath(key string) string {
	k := strings.TrimLeft(strings.TrimSpace(key), "/")

	if s == nil || strings.TrimSpace(s.basePath) == "" {
		return k
	}

	return path.Join(s.basePath, k)
}

func (s *S3Uploader) PublicBaseURL() string {
	if s == nil {
		return ""
	}

	return strings.TrimRight(strings.TrimSpace(s.publicBaseURL), "/")
}

func (s *S3Uploader) Bucket() string {
	if s == nil {
		return ""
	}

	return strings.TrimSpace(s.bucket)
}

func (s *S3Uploader) ProviderName() string {
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

	rawParts := strings.Split(f, "/")
	cleanParts := make([]string, 0, len(rawParts))

	for _, part := range rawParts {
		part = strings.TrimSpace(part)

		if part == "" || part == "." || part == ".." || strings.Contains(part, "..") {
			return "", errors.New("invalid folder")
		}

		clean := s3SanitizeSegment(part)
		if clean == "" || clean == "unknown" {
			return "", errors.New("invalid folder")
		}

		cleanParts = append(cleanParts, clean)
	}

	if len(cleanParts) == 0 {
		return "uploads", nil
	}

	return path.Join(cleanParts...), nil
}

func s3SanitizeSegment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "unknown"
	}

	var b strings.Builder

	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}

	clean := strings.Trim(b.String(), "-_")
	if clean == "" {
		return "unknown"
	}

	for strings.Contains(clean, "--") {
		clean = strings.ReplaceAll(clean, "--", "-")
	}

	return clean
}

func s3SanitizeExtension(ext string) string {
	ext = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".")
	if ext == "" {
		return ""
	}

	var b strings.Builder

	for _, r := range ext {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}

	return b.String()
}

func normalizeEndpoint(endpoint string) string {
	raw := strings.TrimSpace(endpoint)
	if raw == "" {
		return ""
	}

	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return strings.TrimRight(raw, "/")
	}

	return "https://" + strings.TrimRight(raw, "/")
}

func derivePublicBaseURL(bucket, endpoint, region string) string {
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return ""
	}

	endpoint = normalizeEndpoint(endpoint)

	if endpoint != "" {
		u, err := url.Parse(endpoint)
		if err == nil {
			host := strings.TrimSpace(u.Host)

			if host == "" {
				host = strings.TrimSpace(u.Path)
			}

			scheme := strings.TrimSpace(u.Scheme)
			if scheme == "" {
				scheme = "https"
			}

			// Supabase S3 endpoint:
			// https://<project-ref>.storage.supabase.co/storage/v1/s3
			//
			// Public object URL:
			// https://<project-ref>.supabase.co/storage/v1/object/public/<bucket>/<key>
			if strings.Contains(strings.ToLower(host), ".storage.supabase.co") {
				projectRef := strings.TrimSuffix(host, ".storage.supabase.co")
				if projectRef != "" {
					return fmt.Sprintf(
						"%s://%s.supabase.co/storage/v1/object/public/%s",
						scheme,
						projectRef,
						url.PathEscape(bucket),
					)
				}
			}

			if host != "" {
				if !strings.HasPrefix(host, bucket+".") {
					host = bucket + "." + host
				}

				return scheme + "://" + host
			}
		}
	}

	if region != "" {
		return fmt.Sprintf("https://%s.s3.%s.amazonaws.com", bucket, region)
	}

	return ""
}

func joinPublicURL(baseURL, key string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	key = strings.TrimLeft(strings.TrimSpace(key), "/")

	if baseURL == "" {
		return key
	}

	if key == "" {
		return baseURL
	}

	parts := strings.Split(key, "/")
	escaped := make([]string, 0, len(parts))

	for _, part := range parts {
		if part == "" {
			continue
		}

		escaped = append(escaped, url.PathEscape(part))
	}

	if len(escaped) == 0 {
		return baseURL
	}

	return baseURL + "/" + strings.Join(escaped, "/")
}

func shouldDisableObjectACL(endpoint, provider string) bool {
	provider = strings.ToLower(strings.TrimSpace(provider))

	if provider == "supabase" || provider == "supabase-storage" {
		return true
	}

	return isSupabaseStorageS3Endpoint(endpoint)
}

func isSupabaseStorageS3Endpoint(endpoint string) bool {
	lower := strings.ToLower(strings.TrimSpace(endpoint))

	return strings.Contains(lower, ".storage.supabase.co/storage/v1/s3") ||
		strings.Contains(lower, ".supabase.co/storage/v1/s3")
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

func firstEnv(keys ...string) string {
	for _, key := range keys {
		val := strings.TrimSpace(os.Getenv(key))
		if val != "" {
			return val
		}
	}

	return ""
}
