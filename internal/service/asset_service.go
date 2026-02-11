package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

type AssetService interface {
	PresignUpload(req *models.PresignAssetRequest, createdBy *string) (*models.PresignAssetResponse, error)
	CompleteUpload(id string) (*models.Asset, error)
	GetByID(id string) (*models.Asset, error)
	List(page, limit int, ownerType, ownerID string) ([]models.Asset, int64, error)
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

	ownerType := ""
	if req.OwnerType != nil {
		ownerType = strings.ToLower(strings.TrimSpace(*req.OwnerType))
	}
	ownerID := ""
	if req.OwnerID != nil {
		ownerID = strings.TrimSpace(*req.OwnerID)
	}
	kind := ""
	if req.Kind != nil {
		kind = strings.ToLower(strings.TrimSpace(*req.Kind))
	}
	folder := ""
	if req.Folder != nil {
		folder = strings.TrimSpace(*req.Folder)
	}

	objectKey, err := s.buildObjectKey(ownerType, ownerID, kind, folder, ext)
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

	publicBase := resolveSpacesPublicBaseURL(s.uploader)
	if publicBase == "" {
		return nil, errors.New("public url base not configured")
	}
	publicURL := strings.TrimRight(publicBase, "/") + "/" + strings.TrimLeft(objectKey, "/")
	if publicURL == "" {
		return nil, errors.New("public url base not configured")
	}

	asset := &models.Asset{
		OwnerType:   nilIfEmpty(ownerType),
		OwnerID:     nilIfEmpty(ownerID),
		Kind:        nilIfEmpty(kind),
		Provider:    "spaces",
		Bucket:      strings.TrimSpace(os.Getenv("SPACES_BUCKET")),
		ObjectKey:   objectKey,
		PublicURL:   publicURL,
		ContentType: nilIfEmpty(ct),
		SizeBytes:   req.SizeBytes,
		Checksum:    req.Checksum,
		Status:      models.AssetStatusPending,
		CreatedByID: createdBy,
	}

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

func (s *assetService) CompleteUpload(id string) (*models.Asset, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("asset id is required")
	}
	if err := s.repo.SetStatus(id, models.AssetStatusReady); err != nil {
		return nil, err
	}
	return s.repo.GetByID(id)
}

func (s *assetService) GetByID(id string) (*models.Asset, error) {
	if strings.TrimSpace(id) == "" {
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
	offset := (page - 1) * limit
	return s.repo.List(offset, limit, ownerType, ownerID)
}

func (s *assetService) buildObjectKey(ownerType, ownerID, kind, folder, ext string) (string, error) {
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
	case "image/jpeg":
		return "jpg", nil
	case "image/jpg":
		return "jpg", nil
	case "image/webp":
		return "webp", nil
	case "image/gif":
		return "gif", nil
	case "application/pdf":
		return "pdf", nil
	default:
		return "", fmt.Errorf("unsupported content type: %s", ct)
	}
}

func resolveSpacesPublicBaseURL(uploader AssetUploader) string {
	if uploader != nil {
		if spaces, ok := uploader.(*SpacesUploader); ok {
			if base := strings.TrimRight(strings.TrimSpace(spaces.publicBaseURL), "/"); base != "" {
				return base
			}
		}
		if provider, ok := uploader.(interface{ PublicBaseURL() string }); ok {
			if base := strings.TrimRight(strings.TrimSpace(provider.PublicBaseURL()), "/"); base != "" {
				return base
			}
		}
	}
	return strings.TrimRight(strings.TrimSpace(os.Getenv("SPACES_PUBLIC_BASE_URL")), "/")
}

func nilIfEmpty(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}
