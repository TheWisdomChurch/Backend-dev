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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"
)

// SpacesUploader implements AssetUploader using S3-compatible storage (DigitalOcean Spaces).
type SpacesUploader struct {
	bucket        string
	basePath      string
	publicBaseURL string
	publicRead    bool
	client        *s3.Client
}

// NewSpacesUploaderFromEnv builds an uploader from SPACES_* env vars.
// Returns (nil, nil) when no Spaces configuration is present.
func NewSpacesUploaderFromEnv() (*SpacesUploader, error) {
	bucket := strings.TrimSpace(os.Getenv("SPACES_BUCKET"))
	accessKey := strings.TrimSpace(os.Getenv("SPACES_ACCESS_KEY"))
	secretKey := strings.TrimSpace(os.Getenv("SPACES_SECRET_KEY"))
	endpoint := strings.TrimSpace(os.Getenv("SPACES_ENDPOINT"))
	region := strings.TrimSpace(os.Getenv("SPACES_REGION"))
	publicBaseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("SPACES_PUBLIC_BASE_URL")), "/")
	basePath := strings.Trim(strings.TrimSpace(os.Getenv("SPACES_BASE_PATH")), "/")

	if bucket == "" && accessKey == "" && secretKey == "" && endpoint == "" && publicBaseURL == "" && region == "" {
		return nil, nil
	}

	if bucket == "" || accessKey == "" || secretKey == "" {
		return nil, errors.New("spaces config incomplete: require SPACES_BUCKET, SPACES_ACCESS_KEY, SPACES_SECRET_KEY")
	}

	if region == "" {
		region = "us-east-1"
	}

	endpoint = normalizeEndpoint(endpoint)
	if publicBaseURL == "" {
		publicBaseURL = derivePublicBaseURL(bucket, endpoint, region)
	}
	if publicBaseURL == "" {
		return nil, errors.New("SPACES_PUBLIC_BASE_URL is required to build public URLs")
	}

	publicRead := parseBoolEnv("SPACES_PUBLIC_READ", true)

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
