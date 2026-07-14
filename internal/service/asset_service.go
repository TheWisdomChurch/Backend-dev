package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

type AssetService interface {
	PresignUpload(req *models.PresignAssetRequest, createdBy *string) (*models.PresignAssetResponse, error)
	CompleteUpload(id string) (*models.Asset, error)
	GetByID(id string) (*models.Asset, error)
	List(page, limit int, ownerType, ownerID string) ([]models.Asset, int64, error)
	RecordUploadedAsset(req *models.RecordUploadedAssetRequest, createdBy *string) (*models.Asset, error)
	// UpdateProcessingResult is called by the async video worker once
	// transcoding finishes (or fails) to move the asset out of "pending" and
	// merge in the derived output — the transcoded URL on success, or an
	// error note on failure. Existing metadata (poster, original, probed
	// dimensions) is preserved, not overwritten.
	UpdateProcessingResult(id string, status models.AssetStatus, metadataPatch map[string]any) error
}

type presignCapable interface {
	PresignPut(ctx context.Context, objectKey string, contentType string, expires time.Duration) (string, error)
}

type assetService struct {
	repo     repository.AssetRepository
	uploader AssetUploader
}

func NewAssetService(repo repository.AssetRepository, uploader AssetUploader) AssetService {
	return &assetService{
		repo:     repo,
		uploader: uploader,
	}
}

func (s *assetService) PresignUpload(req *models.PresignAssetRequest, createdBy *string) (*models.PresignAssetResponse, error) {
	if s.uploader == nil {
		return nil, errors.New("storage uploader not configured")
	}
	if req == nil {
		return nil, errors.New("request is required")
	}

	ct := strings.TrimSpace(req.ContentType)
	if ct == "" {
		return nil, errors.New("contentType is required")
	}

	ext, err := extFromContentType(ct)
	if err != nil {
		return nil, err
	}

	ownerType := strings.ToLower(stringPtrTrim(req.OwnerType))
	ownerIDRaw := stringPtrTrim(req.OwnerID)
	ownerID, ownerIDMetadata := normalizeAssetOwnerID(ownerIDRaw)

	kind := strings.ToLower(stringPtrTrim(req.Kind))
	folder := stringPtrTrim(req.Folder)

	objectKey, err := s.buildObjectKey(ownerType, ownerIDRaw, kind, folder, ext)
	if err != nil {
		return nil, err
	}

	presigner, ok := s.uploader.(presignCapable)
	if !ok {
		return nil, errors.New("uploader does not support presigning")
	}

	expiry := 15 * time.Minute
	uploadURL, err := presigner.PresignPut(context.Background(), objectKey, ct, expiry)
	if err != nil {
		return nil, err
	}

	publicBase := resolveUploaderPublicBaseURL(s.uploader)
	if publicBase == "" {
		return nil, errors.New("public url base not configured")
	}

	publicURL := strings.TrimRight(publicBase, "/") + "/" + strings.TrimLeft(objectKey, "/")

	metadata := map[string]any{}
	if folder != "" {
		metadata["folder"] = folder
	}
	for k, v := range ownerIDMetadata {
		metadata[k] = v
	}

	asset := &models.Asset{
		OwnerType:   nilIfEmpty(ownerType),
		OwnerID:     ownerID,
		Kind:        nilIfEmpty(kind),
		Provider:    resolveUploaderProvider(s.uploader),
		Bucket:      resolveUploaderBucket(s.uploader),
		ObjectKey:   objectKey,
		PublicURL:   publicURL,
		ContentType: nilIfEmpty(ct),
		SizeBytes:   req.SizeBytes,
		Checksum:    req.Checksum,
		Status:      models.AssetStatusPending,
		CreatedByID: createdBy,
	}

	applyAssetMetadata(asset, metadata)

	if err := s.repo.Create(asset); err != nil {
		return nil, err
	}

	return &models.PresignAssetResponse{
		AssetID:   asset.ID,
		UploadURL: uploadURL,
		ObjectKey: objectKey,
		PublicURL: publicURL,
	}, nil
}

func (s *assetService) RecordUploadedAsset(req *models.RecordUploadedAssetRequest, createdBy *string) (*models.Asset, error) {
	if s.uploader == nil {
		return nil, errors.New("storage uploader not configured")
	}
	if req == nil {
		return nil, errors.New("request is required")
	}

	objectKey := strings.TrimSpace(req.ObjectKey)
	if objectKey == "" {
		return nil, errors.New("objectKey is required")
	}

	publicURL := strings.TrimSpace(req.PublicURL)
	if publicURL == "" {
		return nil, errors.New("publicUrl is required")
	}

	ownerType := strings.ToLower(stringPtrTrim(req.OwnerType))
	ownerIDRaw := stringPtrTrim(req.OwnerID)
	ownerID, ownerIDMetadata := normalizeAssetOwnerID(ownerIDRaw)

	kind := strings.ToLower(stringPtrTrim(req.Kind))
	folder := stringPtrTrim(req.Folder)
	originalName := stringPtrTrim(req.OriginalName)

	metadata := map[string]any{}
	if folder != "" {
		metadata["folder"] = folder
	}
	if originalName != "" {
		metadata["originalName"] = originalName
	}
	for k, v := range ownerIDMetadata {
		metadata[k] = v
	}
	for k, v := range req.Metadata {
		metadata[k] = v
	}

	asset := &models.Asset{
		OwnerType:   nilIfEmpty(ownerType),
		OwnerID:     ownerID,
		Kind:        nilIfEmpty(kind),
		Provider:    resolveUploaderProvider(s.uploader),
		Bucket:      resolveUploaderBucket(s.uploader),
		ObjectKey:   objectKey,
		PublicURL:   publicURL,
		ContentType: nilIfEmpty(req.ContentType),
		SizeBytes:   &req.SizeBytes,
		Checksum:    req.Checksum,
		Status:      models.AssetStatusReady,
		CreatedByID: createdBy,
	}
	if req.Status != nil {
		asset.Status = *req.Status
	}

	applyAssetMetadata(asset, metadata)

	if err := s.repo.Create(asset); err != nil {
		return nil, err
	}

	return asset, nil
}

func (s *assetService) UpdateProcessingResult(id string, status models.AssetStatus, metadataPatch map[string]any) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("asset id is required")
	}

	asset, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}

	existing := map[string]any{}
	if len(asset.Metadata) > 0 {
		_ = json.Unmarshal(asset.Metadata, &existing)
	}
	for k, v := range metadataPatch {
		existing[k] = v
	}
	applyAssetMetadata(asset, existing)
	asset.Status = status

	if url, ok := metadataPatch["transcodedUrl"].(string); ok && strings.TrimSpace(url) != "" {
		asset.PublicURL = url
	}

	return s.repo.Update(asset)
}

func (s *assetService) CompleteUpload(id string) (*models.Asset, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("asset id is required")
	}

	if err := s.repo.SetStatus(id, models.AssetStatusReady); err != nil {
		return nil, err
	}

	return s.repo.GetByID(id)
}

func (s *assetService) GetByID(id string) (*models.Asset, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("asset id is required")
	}

	return s.repo.GetByID(id)
}

func (s *assetService) List(page, limit int, ownerType, ownerID string) ([]models.Asset, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}

	ownerType = strings.ToLower(strings.TrimSpace(ownerType))
	ownerID = strings.TrimSpace(ownerID)

	// assets.owner_id is UUID in the database.
	// If someone filters with a label like "Anniversary", do not query owner_id with it.
	if ownerID != "" {
		if _, err := uuid.Parse(ownerID); err != nil {
			ownerID = ""
		}
	}

	offset := (page - 1) * limit
	return s.repo.List(offset, limit, ownerType, ownerID)
}

func (s *assetService) buildObjectKey(ownerType, ownerID, kind, folder, ext string) (string, error) {
	ownerType = strings.ToLower(strings.TrimSpace(ownerType))
	ownerID = strings.TrimSpace(ownerID)
	kind = strings.ToLower(strings.TrimSpace(kind))
	folder = strings.TrimSpace(folder)

	switch ownerType {
	case "event":
		if ownerID == "" {
			return "", errors.New("ownerId is required for event assets")
		}
		if kind == "" {
			kind = "image"
		}
		return s.uploader.BuildEventAssetKey(ownerID, kind, ext)

	case "testimonial":
		return s.uploader.BuildTestimonialImageKey(ext)

	default:
		if folder == "" {
			folder = "uploads"
		}
		return s.uploader.BuildGenericAssetKey(folder, ext)
	}
}

func extFromContentType(ct string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(ct)) {
	case "image/png":
		return "png", nil
	case "image/jpeg", "image/jpg":
		return "jpg", nil
	case "image/webp":
		return "webp", nil
	case "image/gif":
		return "gif", nil

	case "application/pdf":
		return "pdf", nil

	case "video/mp4":
		return "mp4", nil
	case "video/webm":
		return "webm", nil
	case "video/quicktime":
		return "mov", nil
	case "video/x-msvideo":
		return "avi", nil
	case "video/x-matroska":
		return "mkv", nil

	case "audio/mpeg":
		return "mp3", nil
	case "audio/mp4":
		return "m4a", nil
	case "audio/wav", "audio/x-wav":
		return "wav", nil
	case "audio/webm":
		return "webm", nil
	case "audio/ogg":
		return "ogg", nil

	case "application/msword":
		return "doc", nil
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return "docx", nil
	case "application/vnd.ms-excel":
		return "xls", nil
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return "xlsx", nil
	case "text/csv":
		return "csv", nil
	case "text/plain":
		return "txt", nil

	default:
		return "", fmt.Errorf("unsupported content type: %s", ct)
	}
}

func resolveUploaderPublicBaseURL(uploader AssetUploader) string {
	if uploader != nil {
		if provider, ok := uploader.(interface{ PublicBaseURL() string }); ok {
			if base := strings.TrimRight(strings.TrimSpace(provider.PublicBaseURL()), "/"); base != "" {
				return base
			}
		}
	}

	if base := strings.TrimRight(strings.TrimSpace(os.Getenv("S3_PUBLIC_BASE_URL")), "/"); base != "" {
		return base
	}

	return ""
}

func resolveUploaderBucket(uploader AssetUploader) string {
	if uploader != nil {
		if provider, ok := uploader.(interface{ Bucket() string }); ok {
			if bucket := strings.TrimSpace(provider.Bucket()); bucket != "" {
				return bucket
			}
		}
	}

	if bucket := strings.TrimSpace(os.Getenv("S3_BUCKET")); bucket != "" {
		return bucket
	}

	return ""
}

func resolveUploaderProvider(uploader AssetUploader) string {
	if uploader != nil {
		if provider, ok := uploader.(interface{ ProviderName() string }); ok {
			if name := strings.TrimSpace(provider.ProviderName()); name != "" {
				return strings.ToLower(name)
			}
		}
	}

	for _, key := range []string{"S3_PROVIDER", "STORAGE_PROVIDER"} {
		if name := strings.TrimSpace(os.Getenv(key)); name != "" {
			return strings.ToLower(name)
		}
	}

	return "s3"
}

func normalizeAssetOwnerID(raw string) (*string, map[string]any) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	parsed, err := uuid.Parse(raw)
	if err == nil {
		normalized := parsed.String()
		return &normalized, nil
	}

	// owner_id is a UUID column.
	// Labels like "Anniversary", "homepage", "banner", etc. must not be inserted there.
	// Preserve the label in metadata instead.
	return nil, map[string]any{
		"ownerRef": raw,
	}
}

func applyAssetMetadata(asset *models.Asset, metadata map[string]any) {
	if asset == nil || len(metadata) == 0 {
		return
	}

	clean := map[string]any{}

	for key, value := range metadata {
		key = strings.TrimSpace(key)
		if key == "" || value == nil {
			continue
		}

		if text, ok := value.(string); ok {
			text = strings.TrimSpace(text)
			if text == "" {
				continue
			}
			clean[key] = text
			continue
		}

		clean[key] = value
	}

	if len(clean) == 0 {
		return
	}

	if b, err := json.Marshal(clean); err == nil {
		asset.Metadata = b
	}
}

func stringPtrTrim(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func nilIfEmpty(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}
