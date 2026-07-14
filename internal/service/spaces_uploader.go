package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"
)

// S3Uploader implements AssetUploader using S3-compatible object storage.
//
// Supabase Storage S3 note:
// Supabase uses an S3-compatible endpoint, but it is strict about path-style
// requests and does not support x-amz-acl. For provider=supabase, Upload uses a
// direct AWS Signature V4 signed HTTP PUT. This mirrors the AWS CLI request shape
// that works against Supabase Storage S3.
type S3Uploader struct {
	bucket         string
	basePath       string
	publicBaseURL  string
	publicRead     bool
	provider       string
	endpoint       string
	region         string
	accessKey      string
	secretKey      string
	forcePathStyle bool
	client         *s3.Client
}

// NewS3UploaderFromEnv builds an uploader from S3_* env vars.
//
// Required for Supabase:
//
//	S3_PROVIDER=supabase
//	S3_BUCKET=wisdomchurch-media
//	S3_REGION=eu-west-1
//	S3_ENDPOINT=https://<project-ref>.storage.supabase.co/storage/v1/s3
//	S3_ACCESS_KEY=<Storage S3 Access Key ID>
//	S3_SECRET_KEY=<Storage S3 Secret Access Key>
//	S3_PUBLIC_BASE_URL=https://<project-ref>.supabase.co/storage/v1/object/public/<bucket>
//	S3_PUBLIC_READ=false
//	S3_BASE_PATH=uploads
func NewS3UploaderFromEnv() (*S3Uploader, error) {
	bucket := strings.TrimSpace(firstEnv("S3_BUCKET"))
	accessKey := strings.TrimSpace(firstEnv("S3_ACCESS_KEY"))
	secretKey := strings.TrimSpace(firstEnv("S3_SECRET_KEY"))
	endpoint := normalizeS3Endpoint(firstEnv("S3_ENDPOINT"))
	region := strings.TrimSpace(firstEnv("S3_REGION"))
	publicBaseURL := strings.TrimRight(strings.TrimSpace(firstEnv("S3_PUBLIC_BASE_URL")), "/")
	basePath := strings.Trim(firstEnv("S3_BASE_PATH"), "/")
	provider := strings.ToLower(strings.TrimSpace(firstEnv("S3_PROVIDER", "STORAGE_PROVIDER")))

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
	if endpoint == "" {
		return nil, errors.New("storage config incomplete: S3_ENDPOINT is required")
	}
	if region == "" {
		region = "us-east-1"
	}

	if publicBaseURL == "" {
		publicBaseURL = deriveS3PublicBaseURL(bucket, endpoint)
	}
	if publicBaseURL == "" {
		return nil, errors.New("S3_PUBLIC_BASE_URL is required to build public URLs")
	}

	// Supabase Storage S3 rejects x-amz-acl. Keep false for Supabase.
	publicRead := parseBoolEnv("S3_PUBLIC_READ", false)
	forcePathStyle := true

	loadOpts := []func(*config.LoadOptions) error{
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	}

	resolver := aws.EndpointResolverWithOptionsFunc(
		func(service, resolverRegion string, options ...interface{}) (aws.Endpoint, error) {
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

	cfg, err := config.LoadDefaultConfig(context.Background(), loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("s3 config load failed: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = forcePathStyle
	})

	return &S3Uploader{
		bucket:         bucket,
		basePath:       basePath,
		publicBaseURL:  publicBaseURL,
		publicRead:     publicRead,
		provider:       provider,
		endpoint:       endpoint,
		region:         region,
		accessKey:      accessKey,
		secretKey:      secretKey,
		forcePathStyle: forcePathStyle,
		client:         client,
	}, nil
}

// StorageSummary returns a safe, non-secret startup summary for logs.
// main.go calls this method, so keep it stable.
func (s *S3Uploader) StorageSummary() string {
	if s == nil {
		return "provider=none configured=false"
	}

	return fmt.Sprintf(
		"provider=%s bucket=%s region=%s endpoint=%s publicBaseURL=%s forcePathStyle=%t publicRead=%t supportsACL=%t accessKeySuffix=%s",
		s.ProviderName(),
		s.Bucket(),
		s.Region(),
		s.Endpoint(),
		s.PublicBaseURL(),
		s.ForcePathStyle(),
		s.publicRead,
		s.SupportsACL(),
		safeSuffix(s.accessKey, 6),
	)
}

func (s *S3Uploader) Upload(ctx context.Context, objectKey string, contentType string, r io.Reader) (string, error) {
	if s == nil {
		return "", errors.New("uploader not configured")
	}

	key := strings.TrimLeft(strings.TrimSpace(objectKey), "/")
	if key == "" {
		return "", errors.New("object key is required")
	}

	if s.isSupabase() {
		return s.uploadSupabaseSigV4(ctx, key, contentType, r)
	}

	if s.client == nil {
		return "", errors.New("uploader client not configured")
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
			"s3 PutObject failed: bucket=%q key=%q contentType=%q endpoint=%q publicRead=%t supportsACL=%t forcePathStyle=%t accessKeySuffix=%q: %w",
			s.bucket,
			key,
			ct,
			s.endpoint,
			s.publicRead,
			s.supportsACL(),
			s.forcePathStyle,
			safeSuffix(s.accessKey, 6),
			err,
		)
	}

	return s.publicURLForKey(key), nil
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
			"s3 PresignPutObject failed: bucket=%q key=%q contentType=%q endpoint=%q forcePathStyle=%t accessKeySuffix=%q: %w",
			s.bucket,
			key,
			ct,
			s.endpoint,
			s.forcePathStyle,
			safeSuffix(s.accessKey, 6),
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

	key := path.Join("events", sanitizeS3Segment(eventID), fmt.Sprintf("%s.%s", sanitizeS3Segment(kind), ext))
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

	folder, err := sanitizeS3Folder(folder)
	if err != nil {
		return "", err
	}

	key := path.Join(folder, uuid.NewString()+"."+ext)
	return s.withBasePath(key), nil
}

// BuildImageVariantKey builds one key in a shared-assetID group — the
// original plus each derived size (see ImageProcessor) all live under
// folder/assetID/, so a processed image and all its variants are trivially
// discoverable as a set instead of being scattered under unrelated random
// keys.
func (s *S3Uploader) BuildImageVariantKey(folder, assetID, variant, ext string) (string, error) {
	ext = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".")
	if ext == "" {
		return "", errors.New("ext is required")
	}
	assetID = sanitizeS3Segment(assetID)
	variant = sanitizeS3Segment(variant)
	if assetID == "" || variant == "" {
		return "", errors.New("assetID and variant are required")
	}

	folder, err := sanitizeS3Folder(folder)
	if err != nil {
		return "", err
	}

	key := path.Join(folder, assetID, variant+"."+ext)
	return s.withBasePath(key), nil
}

// NewAssetID generates the shared ID BuildImageVariantKey groups variants
// under. A plain function would do, but keeping ID generation behind the
// uploader keeps callers from needing their own uuid import just for this.
func (s *S3Uploader) NewAssetID() string {
	return uuid.NewString()
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

func (s *S3Uploader) Endpoint() string {
	if s == nil {
		return ""
	}

	return strings.TrimSpace(s.endpoint)
}

func (s *S3Uploader) Region() string {
	if s == nil {
		return ""
	}

	return strings.TrimSpace(s.region)
}

func (s *S3Uploader) ForcePathStyle() bool {
	if s == nil {
		return false
	}

	return s.forcePathStyle
}

func (s *S3Uploader) SupportsACL() bool {
	if s == nil {
		return false
	}

	return s.supportsACL()
}

func (s *S3Uploader) isSupabase() bool {
	if s == nil {
		return false
	}

	lowerEndpoint := strings.ToLower(strings.TrimSpace(s.endpoint))
	lowerProvider := strings.ToLower(strings.TrimSpace(s.provider))

	return lowerProvider == "supabase" ||
		strings.Contains(lowerEndpoint, "supabase.co") ||
		strings.Contains(lowerEndpoint, "/storage/v1/s3")
}

func (s *S3Uploader) supportsACL() bool {
	if s == nil {
		return false
	}

	return !s.isSupabase()
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

func (s *S3Uploader) publicURLForKey(key string) string {
	return strings.TrimRight(s.publicBaseURL, "/") + "/" + strings.TrimLeft(key, "/")
}

func sanitizeS3Folder(folder string) (string, error) {
	f := strings.Trim(strings.TrimSpace(folder), "/")
	if f == "" {
		return "uploads", nil
	}

	parts := strings.Split(f, "/")
	cleanParts := make([]string, 0, len(parts))

	for _, part := range parts {
		clean := sanitizeS3Segment(part)
		if clean == "" || clean == "." || clean == ".." {
			return "", errors.New("invalid folder")
		}

		cleanParts = append(cleanParts, clean)
	}

	return path.Join(cleanParts...), nil
}

func sanitizeS3Segment(value string) string {
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

func normalizeS3Endpoint(endpoint string) string {
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

func deriveS3PublicBaseURL(bucket, endpoint string) string {
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return ""
	}

	endpoint = normalizeS3Endpoint(endpoint)
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

/* =========================
   Supabase direct SigV4 PUT
========================= */

// uploadSupabaseSigV4 signs a raw HTTP PUT with the AWS SDK's own v4.Signer
// rather than the S3 client's PutObject.
//
// This is deliberately NOT s.client.PutObject: newer aws-sdk-go-v2 S3 clients
// default to streaming uploads with trailing checksums ("aws-chunked" transfer
// encoding, x-amz-checksum-*), which Supabase Storage's S3-compatible gateway
// does not implement — those requests fail before Supabase ever validates the
// signature. A plain, fully-buffered http.Request sidesteps that entirely.
// What this function used to also do — hand-roll the canonical request and
// HMAC chain by hand — bought nothing: v4.Signer.SignHTTP is the exact same
// signer the SDK itself uses internally, so it produces an identical
// Authorization header without ~150 lines of unverified hand-written crypto.
func (s *S3Uploader) uploadSupabaseSigV4(ctx context.Context, key string, contentType string, r io.Reader) (string, error) {
	if s == nil {
		return "", errors.New("uploader not configured")
	}
	if strings.TrimSpace(s.accessKey) == "" {
		return "", errors.New("S3_ACCESS_KEY is required")
	}
	if strings.TrimSpace(s.secretKey) == "" {
		return "", errors.New("S3_SECRET_KEY is required")
	}
	if strings.TrimSpace(s.endpoint) == "" {
		return "", errors.New("S3_ENDPOINT is required")
	}
	if strings.TrimSpace(s.bucket) == "" {
		return "", errors.New("S3_BUCKET is required")
	}

	payload, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("failed to read upload payload: %w", err)
	}

	endpointURL, err := url.Parse(s.endpoint)
	if err != nil {
		return "", fmt.Errorf("invalid S3_ENDPOINT %q: %w", s.endpoint, err)
	}

	endpointURL.Path = joinS3URLPath(endpointURL.Path, s.bucket, key)
	endpointURL.RawQuery = ""
	endpointURL.RawPath = ""

	region := strings.TrimSpace(s.region)
	if region == "" {
		region = "us-east-1"
	}

	payloadHash := sha256Hex(payload)
	ct := strings.TrimSpace(contentType)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpointURL.String(), bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("failed to build upload request: %w", err)
	}
	req.Host = endpointURL.Host
	req.ContentLength = int64(len(payload))
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	// SignHTTP only uses payloadHash to compute the signature — it does not
	// add this as a request header. S3(-compatible) servers re-derive the
	// canonical request from the headers they actually receive, so this must
	// be set on the wire request too, or the server-side signature check
	// fails even though the client-side signing "succeeded".
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	// Supabase Storage's S3-compatible gateway rejects ACL headers outright.
	if s.publicRead && s.supportsACL() {
		req.Header.Set("X-Amz-Acl", "public-read")
	}

	credentials := aws.Credentials{
		AccessKeyID:     s.accessKey,
		SecretAccessKey: s.secretKey,
	}
	if err := v4.NewSigner().SignHTTP(ctx, credentials, req, payloadHash, "s3", region, time.Now().UTC()); err != nil {
		return "", fmt.Errorf("failed to sign upload request: %w", err)
	}

	client := &http.Client{Timeout: 120 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf(
			"supabase signed PutObject failed before response: bucket=%q key=%q endpoint=%q region=%q accessKeySuffix=%q: %w",
			s.bucket,
			key,
			s.endpoint,
			region,
			safeSuffix(s.accessKey, 6),
			err,
		)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return "", fmt.Errorf(
			"supabase signed PutObject failed: status=%d bucket=%q key=%q endpoint=%q region=%q accessKeySuffix=%q body=%s",
			resp.StatusCode,
			s.bucket,
			key,
			s.endpoint,
			region,
			safeSuffix(s.accessKey, 6),
			strings.TrimSpace(string(bodyBytes)),
		)
	}

	return s.publicURLForKey(key), nil
}

func joinS3URLPath(basePath string, bucket string, key string) string {
	base := strings.Trim(basePath, "/")
	bucket = strings.Trim(bucket, "/")
	key = strings.TrimLeft(strings.TrimSpace(key), "/")

	if base == "" {
		return "/" + path.Join(bucket, key)
	}

	return "/" + path.Join(base, bucket, key)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func safeSuffix(value string, count int) string {
	value = strings.TrimSpace(value)
	if count <= 0 || value == "" {
		return ""
	}
	if len(value) <= count {
		return value
	}

	return value[len(value)-count:]
}
