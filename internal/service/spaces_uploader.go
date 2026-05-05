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

// S3Uploader implements AssetUploader using AWS S3 or S3-compatible storage.
type S3Uploader struct {
	bucket        string
	basePath      string
	publicBaseURL string
	publicRead    bool
	provider      string
	endpoint      string
	region        string
	forcePath     bool
	client        *s3.Client
}

func NewS3UploaderFromEnv() (*S3Uploader, error) {
	bucket := firstEnv("S3_BUCKET")
	accessKey := firstEnv("S3_ACCESS_KEY", "AWS_ACCESS_KEY_ID")
	secretKey := firstEnv("S3_SECRET_KEY", "AWS_SECRET_ACCESS_KEY")
	endpoint := normalizeEndpoint(firstEnv("S3_ENDPOINT"))
	region := firstEnv("S3_REGION", "AWS_REGION", "AWS_DEFAULT_REGION")
	publicBaseURL := strings.TrimRight(firstEnv("S3_PUBLIC_BASE_URL"), "/")
	basePath := strings.Trim(firstEnv("S3_BASE_PATH"), "/")
	provider := strings.ToLower(firstEnv("S3_PROVIDER", "STORAGE_PROVIDER"))

	if provider == "" {
		provider = "s3"
	}

	if bucket == "" && accessKey == "" && secretKey == "" && endpoint == "" && publicBaseURL == "" && region == "" {
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
		publicBaseURL = derivePublicBaseURL(bucket, endpoint, region, provider)
	}
	if publicBaseURL == "" {
		return nil, errors.New("S3_PUBLIC_BASE_URL is required to build public URLs")
	}

	publicRead := parseBoolEnv("S3_PUBLIC_READ", false)

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
						SigningRegion:     region,
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

	forcePath := shouldForcePathStyle(provider, endpoint)
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = forcePath
	})

	return &S3Uploader{
		bucket:        bucket,
		basePath:      basePath,
		publicBaseURL: publicBaseURL,
		publicRead:    publicRead,
		provider:      provider,
		endpoint:      endpoint,
		region:        region,
		forcePath:     forcePath,
		client:        client,
	}, nil
}

func (s *S3Uploader) Upload(ctx context.Context, objectKey string, contentType string, r io.Reader) (string, error) {
	if s == nil || s.client == nil {
		return "", errors.New("uploader not configured")
	}

	key := strings.TrimLeft(strings.TrimSpace(objectKey), "/")
	if key == "" {
		return "", errors.New("object key is required")
	}

	ct := strings.TrimSpace(contentType)

	input := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   r,
	}

	if ct != "" {
		input.ContentType = aws.String(ct)
	}

	if s.publicRead && s.supportsACL() {
		input.ACL = types.ObjectCannedACLPublicRead
	}

	if _, err := s.client.PutObject(ctx, input); err != nil {
		return "", fmt.Errorf(
			"s3 PutObject failed: bucket=%q key=%q contentType=%q endpoint=%q publicRead=%t supportsACL=%t forcePathStyle=%t: %w",
			s.bucket,
			key,
			ct,
			s.endpointDescription(),
			s.publicRead,
			s.supportsACL(),
			s.forcePath,
			err,
		)
	}

	return strings.TrimRight(s.publicBaseURL, "/") + "/" + key, nil
}

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

	ct := strings.TrimSpace(contentType)

	input := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}

	if ct != "" {
		input.ContentType = aws.String(ct)
	}

	if s.publicRead && s.supportsACL() {
		input.ACL = types.ObjectCannedACLPublicRead
	}

	presigner := s3.NewPresignClient(s.client)

	out, err := presigner.PresignPutObject(ctx, input, s3.WithPresignExpires(expires))
	if err != nil {
		return "", fmt.Errorf(
			"s3 PresignPutObject failed: bucket=%q key=%q contentType=%q endpoint=%q forcePathStyle=%t: %w",
			s.bucket,
			key,
			ct,
			s.endpointDescription(),
			s.forcePath,
			err,
		)
	}

	return out.URL, nil
}

func (s *S3Uploader) BuildEventAssetKey(eventID, kind, ext string) (string, error) {
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

	key := path.Join("events", sanitizeStorageSegment(eventID), fmt.Sprintf("%s.%s", sanitizeStorageSegment(kind), ext))
	return s.withBasePath(key), nil
}

func (s *S3Uploader) BuildTestimonialImageKey(ext string) (string, error) {
	ext = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".")
	if ext == "" {
		return "", errors.New("ext is required")
	}

	key := path.Join("testimonials", uuid.NewString()+"."+ext)
	return s.withBasePath(key), nil
}

func (s *S3Uploader) BuildGenericAssetKey(folder, ext string) (string, error) {
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

func (s *S3Uploader) StorageSummary() string {
	if s == nil {
		return ""
	}

	return fmt.Sprintf(
		"provider=%s bucket=%s region=%s endpoint=%s publicBaseURL=%s forcePathStyle=%t publicRead=%t supportsACL=%t",
		s.ProviderName(),
		s.Bucket(),
		strings.TrimSpace(s.region),
		s.endpointDescription(),
		s.PublicBaseURL(),
		s.forcePath,
		s.publicRead,
		s.supportsACL(),
	)
}

func (s *S3Uploader) endpointDescription() string {
	if s == nil || strings.TrimSpace(s.endpoint) == "" {
		return "aws-default"
	}

	return strings.TrimSpace(s.endpoint)
}

func (s *S3Uploader) supportsACL() bool {
	if s == nil {
		return false
	}

	lowerEndpoint := strings.ToLower(strings.TrimSpace(s.endpoint))
	lowerProvider := strings.ToLower(strings.TrimSpace(s.provider))

	if lowerProvider == "supabase" {
		return false
	}

	if strings.Contains(lowerEndpoint, "supabase.co") {
		return false
	}

	if strings.Contains(lowerEndpoint, "/storage/v1/s3") {
		return false
	}

	return true
}

func (s *S3Uploader) withBasePath(key string) string {
	if s == nil {
		return strings.TrimLeft(strings.TrimSpace(key), "/")
	}

	k := strings.TrimLeft(strings.TrimSpace(key), "/")
	if s.basePath == "" {
		return k
	}

	return path.Join(s.basePath, k)
}

func sanitizeFolder(folder string) (string, error) {
	f := strings.Trim(strings.TrimSpace(folder), "/")
	if f == "" {
		return "uploads", nil
	}

	parts := strings.Split(f, "/")
	cleanParts := make([]string, 0, len(parts))

	for _, part := range parts {
		clean := sanitizeStorageSegment(part)
		if clean == "" || clean == "." || clean == ".." {
			return "", errors.New("invalid folder")
		}

		cleanParts = append(cleanParts, clean)
	}

	return path.Join(cleanParts...), nil
}

func sanitizeStorageSegment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "general"
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
	for strings.Contains(clean, "--") {
		clean = strings.ReplaceAll(clean, "--", "-")
	}

	if clean == "" {
		return "general"
	}

	return clean
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

func shouldForcePathStyle(provider, endpoint string) bool {
	if v := strings.TrimSpace(os.Getenv("S3_FORCE_PATH_STYLE")); v != "" {
		return parseBoolEnv("S3_FORCE_PATH_STYLE", false)
	}

	lowerEndpoint := strings.ToLower(strings.TrimSpace(endpoint))
	lowerProvider := strings.ToLower(strings.TrimSpace(provider))

	if lowerProvider == "aws" || strings.Contains(lowerEndpoint, "amazonaws.com") {
		return false
	}

	return lowerEndpoint != ""
}

func derivePublicBaseURL(bucket, endpoint, region, provider string) string {
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return ""
	}

	region = strings.TrimSpace(region)
	if region == "" {
		region = "us-east-1"
	}

	if endpoint == "" || strings.EqualFold(strings.TrimSpace(provider), "aws") {
		if region == "us-east-1" {
			return "https://" + bucket + ".s3.amazonaws.com"
		}
		return "https://" + bucket + ".s3." + region + ".amazonaws.com"
	}

	endpoint = normalizeEndpoint(endpoint)
	if endpoint == "" {
		return ""
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return ""
	}

	host := u.Host
	if host == "" {
		host = strings.TrimSpace(u.Path)
	}

	scheme := u.Scheme
	if scheme == "" {
		scheme = "https"
	}

	if host == "" {
		return ""
	}

	if strings.Contains(strings.ToLower(host), "supabase.co") {
		projectHost := strings.Replace(host, ".storage.supabase.co", ".supabase.co", 1)
		return scheme + "://" + projectHost + "/storage/v1/object/public/" + bucket
	}

	if strings.Contains(strings.ToLower(host), "amazonaws.com") {
		if region == "us-east-1" {
			return "https://" + bucket + ".s3.amazonaws.com"
		}
		return "https://" + bucket + ".s3." + region + ".amazonaws.com"
	}

	return scheme + "://" + strings.TrimPrefix(host, bucket+".") + "/" + bucket
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
