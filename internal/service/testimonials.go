package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

type TestimonialService interface {
	CreateTestimonial(req *models.CreateTestimonialRequest) (*models.Testimonial, error)
	GetAllTestimonials(approved bool) ([]models.Testimonial, error)
	GetTestimonialByID(id uuid.UUID) (*models.Testimonial, error)
	UpdateTestimonial(id uuid.UUID, req *models.UpdateTestimonialRequest) (*models.Testimonial, error)
	DeleteTestimonial(id uuid.UUID, approver *models.User) error
	GetPaginatedTestimonials(page, limit int, approved bool) ([]models.Testimonial, int64, error)
	ApproveTestimonial(id uuid.UUID, approver *models.User) (*models.Testimonial, error)
}

type testimonialService struct {
	repo            repository.TestimonialRepository
	assetRepo       repository.AssetRepository
	uploader        AssetUploader
	approvalSvc     ApprovalService
	notifySvc       AdminNotificationService
	publicAssetBase string
}

func NewTestimonialService(
	repo repository.TestimonialRepository,
	assetRepo repository.AssetRepository,
	uploader AssetUploader,
	approvalSvc ApprovalService,
	notifySvc AdminNotificationService,
) TestimonialService {
	publicBase := strings.TrimRight(strings.TrimSpace(os.Getenv("S3_PUBLIC_BASE_URL")), "/")
	if publicBase == "" && uploader != nil {
		if provider, ok := uploader.(interface{ PublicBaseURL() string }); ok {
			publicBase = strings.TrimRight(strings.TrimSpace(provider.PublicBaseURL()), "/")
		}
	}

	return &testimonialService{
		repo:            repo,
		assetRepo:       assetRepo,
		uploader:        uploader,
		approvalSvc:     approvalSvc,
		notifySvc:       notifySvc,
		publicAssetBase: publicBase,
	}
}

var (
	ErrInvalidTestimonialInput = errors.New("invalid testimonial input")
	ErrInvalidTestimonialImage = errors.New("invalid testimonial image")
)

const maxUploadedTestimonialImageBytes = 8 * 1024 * 1024 // 8MB

func (s *testimonialService) CreateTestimonial(req *models.CreateTestimonialRequest) (*models.Testimonial, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: request is required", ErrInvalidTestimonialInput)
	}
	req.FirstName = strings.TrimSpace(req.FirstName)
	req.LastName = strings.TrimSpace(req.LastName)
	req.Testimony = strings.TrimSpace(req.Testimony)
	if req.FirstName == "" || req.LastName == "" || req.Testimony == "" {
		return nil, fmt.Errorf("%w: firstName, lastName, and testimony are required", ErrInvalidTestimonialInput)
	}
	imageURL, err := s.maybeUploadImage(req.ImageURL, req.ImageAssetID)
	if err != nil {
		return nil, err
	}

	testimonial := &models.Testimonial{
		FirstName:   req.FirstName,
		LastName:    req.LastName,
		FullName:    fmt.Sprintf("%s %s", req.FirstName, req.LastName),
		ImageURL:    imageURL,
		Testimony:   req.Testimony,
		IsAnonymous: req.IsAnonymous,
		IsApproved:  false,
	}
	if email := strings.TrimSpace(req.Email); email != "" {
		testimonial.ContactEmail = &email
	}
	if phone := strings.TrimSpace(req.Phone); phone != "" {
		testimonial.ContactPhone = &phone
	}

	if err := s.repo.Create(testimonial); err != nil {
		return nil, err
	}

	if s.approvalSvc != nil {
		label := strings.TrimSpace(testimonial.FullName)
		if label == "" {
			label = "Testimonial"
		}
		entityID := testimonial.ID.String()
		req, err := s.approvalSvc.CreateRequest(CreateApprovalRequest{
			Type:        models.ApprovalTypeTestimonial,
			EntityID:    &entityID,
			EntityLabel: &label,
		})
		if err == nil && s.notifySvc != nil {
			title := "New testimonial approval request"
			message := fmt.Sprintf("A new testimonial is awaiting approval. Ticket %s.", req.TicketCode)
			_ = s.notifySvc.NotifyRoles(AdminNotificationInput{
				Type:       "testimonial_request",
				Title:      title,
				Message:    message,
				TicketCode: &req.TicketCode,
				EntityType: func() *string { t := "testimonial"; return &t }(),
				EntityID:   &entityID,
				Roles:      []string{"super_admin"},
			})
		}
	}

	return testimonial, nil
}

func (s *testimonialService) GetAllTestimonials(approved bool) ([]models.Testimonial, error) {
	return s.repo.GetAll(approved)
}

func (s *testimonialService) GetTestimonialByID(id uuid.UUID) (*models.Testimonial, error) {
	return s.repo.GetByID(id)
}

func (s *testimonialService) UpdateTestimonial(id uuid.UUID, req *models.UpdateTestimonialRequest) (*models.Testimonial, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: request is required", ErrInvalidTestimonialInput)
	}
	testimonial, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	// Update fields if provided
	if req.FirstName != nil {
		testimonial.FirstName = *req.FirstName
	}
	if req.LastName != nil {
		testimonial.LastName = *req.LastName
	}
	if req.FirstName != nil || req.LastName != nil {
		testimonial.FullName = fmt.Sprintf("%s %s", testimonial.FirstName, testimonial.LastName)
	}
	if req.ImageURL != nil {
		imageURL, err := s.maybeUploadImage(req.ImageURL, req.ImageAssetID)
		if err != nil {
			return nil, err
		}
		testimonial.ImageURL = imageURL
	} else if req.ImageAssetID != nil {
		imageURL, err := s.maybeUploadImage(nil, req.ImageAssetID)
		if err != nil {
			return nil, err
		}
		testimonial.ImageURL = imageURL
	}
	if req.Testimony != nil {
		testimonial.Testimony = *req.Testimony
	}
	if req.IsAnonymous != nil {
		testimonial.IsAnonymous = *req.IsAnonymous
	}
	if req.IsApproved != nil {
		testimonial.IsApproved = *req.IsApproved
	}

	if err := s.repo.Update(testimonial); err != nil {
		return nil, err
	}

	return testimonial, nil
}

func (s *testimonialService) DeleteTestimonial(id uuid.UUID, approver *models.User) error {
	if err := s.repo.Delete(id); err != nil {
		return err
	}
	if s.approvalSvc != nil {
		_, _ = s.approvalSvc.CompleteRequest(models.ApprovalTypeTestimonial, id.String(), models.ApprovalStatusDeleted, approver)
	}
	if s.notifySvc != nil {
		title := "Testimonial removed"
		message := "A testimonial was removed after review."
		_ = s.notifySvc.NotifyRoles(AdminNotificationInput{
			Type:       "testimonial_deleted",
			Title:      title,
			Message:    message,
			EntityType: func() *string { t := "testimonial"; return &t }(),
			EntityID:   func() *string { v := id.String(); return &v }(),
			Roles:      []string{"admin", "super_admin"},
		})
	}
	return nil
}

func (s *testimonialService) GetPaginatedTestimonials(page, limit int, approved bool) ([]models.Testimonial, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}

	return s.repo.GetPaginated(page, limit, approved)
}

func (s *testimonialService) ApproveTestimonial(id uuid.UUID, approver *models.User) (*models.Testimonial, error) {
	testimonial, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	testimonial.IsApproved = true
	if approver != nil {
		testimonial.ApprovedByID = &approver.ID
		name := strings.TrimSpace(strings.Join([]string{approver.FirstName, approver.LastName}, " "))
		if name != "" {
			testimonial.ApprovedByName = &name
		}
		if approver.Email != "" {
			email := approver.Email
			testimonial.ApprovedByEmail = &email
		}
		now := time.Now().UTC()
		testimonial.ApprovedAt = &now
	}

	if err := s.repo.Update(testimonial); err != nil {
		return nil, err
	}

	if s.approvalSvc != nil {
		_, _ = s.approvalSvc.CompleteRequest(models.ApprovalTypeTestimonial, id.String(), models.ApprovalStatusApproved, approver)
	}
	if s.notifySvc != nil {
		title := "Testimonial approved"
		message := "A testimonial has been approved and published."
		_ = s.notifySvc.NotifyRoles(AdminNotificationInput{
			Type:       "testimonial_approved",
			Title:      title,
			Message:    message,
			EntityType: func() *string { t := "testimonial"; return &t }(),
			EntityID:   func() *string { v := id.String(); return &v }(),
			Roles:      []string{"admin", "super_admin"},
		})
	}

	return testimonial, nil
}

// maybeUploadImage resolves testimonial image from one of:
// 1) imageAssetID (preferred): validates asset state and returns its public URL.
// 2) image string: trusted S3 URL, or base64/dataURL (uploaded to configured S3 uploader).
func (s *testimonialService) maybeUploadImage(image *string, imageAssetID *string) (*string, error) {
	if imageAssetID != nil {
		assetID := strings.TrimSpace(*imageAssetID)
		if assetID != "" {
			assetURL, err := s.resolveAssetPublicURL(assetID)
			if err != nil {
				return nil, err
			}
			return assetURL, nil
		}
	}

	if image == nil {
		return nil, nil
	}

	raw := strings.TrimSpace(*image)
	if raw == "" {
		return nil, nil
	}

	low := strings.ToLower(raw)
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		if !s.isTrustedAssetURL(raw) {
			return nil, fmt.Errorf("%w: external image URL is not allowed", ErrInvalidTestimonialImage)
		}
		return &raw, nil
	}

	if s.uploader == nil {
		return nil, fmt.Errorf("%w: uploader not configured for image content", ErrInvalidTestimonialImage)
	}

	// Expect data URL: data:image/png;base64,.... or plain base64.
	var mimeType string
	var b64data string

	if strings.HasPrefix(low, "data:") {
		parts := strings.SplitN(raw, ",", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("%w: invalid image data URL", ErrInvalidTestimonialImage)
		}
		meta := parts[0]
		b64data = parts[1]
		if strings.Contains(meta, ";") {
			mimeType = strings.TrimPrefix(strings.Split(meta, ";")[0], "data:")
		}
	} else {
		mimeType = "image/png"
		b64data = raw
	}

	if mimeType == "" {
		mimeType = "image/png"
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(mimeType)), "image/") {
		return nil, fmt.Errorf("%w: unsupported image content type", ErrInvalidTestimonialImage)
	}

	decoded, err := base64.StdEncoding.DecodeString(b64data)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid base64 payload", ErrInvalidTestimonialImage)
	}
	if len(decoded) > maxUploadedTestimonialImageBytes {
		return nil, fmt.Errorf("%w: image size exceeds 8MB limit", ErrInvalidTestimonialImage)
	}

	ext := "png"
	if strings.Contains(mimeType, "jpeg") || strings.Contains(mimeType, "jpg") {
		ext = "jpg"
	} else if strings.Contains(mimeType, "webp") {
		ext = "webp"
	}

	key, err := s.uploader.BuildTestimonialImageKey(ext)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	uploadedURL, err := s.uploader.Upload(ctx, key, mimeType, bytes.NewReader(decoded))
	if err != nil {
		return nil, err
	}

	return &uploadedURL, nil
}

func (s *testimonialService) resolveAssetPublicURL(assetID string) (*string, error) {
	if s.assetRepo == nil {
		return nil, fmt.Errorf("%w: asset repository unavailable", ErrInvalidTestimonialImage)
	}
	asset, err := s.assetRepo.GetByID(assetID)
	if err != nil {
		return nil, fmt.Errorf("%w: image asset not found", ErrInvalidTestimonialImage)
	}
	if asset.Status != models.AssetStatusReady {
		return nil, fmt.Errorf("%w: image asset is not ready", ErrInvalidTestimonialImage)
	}
	if asset.ContentType != nil && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(*asset.ContentType)), "image/") {
		return nil, fmt.Errorf("%w: asset is not an image", ErrInvalidTestimonialImage)
	}
	if asset.OwnerType != nil {
		ownerType := strings.ToLower(strings.TrimSpace(*asset.OwnerType))
		if ownerType != "" && ownerType != "testimonial" {
			return nil, fmt.Errorf("%w: asset ownerType must be testimonial", ErrInvalidTestimonialImage)
		}
	}
	publicURL := strings.TrimSpace(asset.PublicURL)
	if publicURL == "" {
		return nil, fmt.Errorf("%w: image asset has no public URL", ErrInvalidTestimonialImage)
	}
	if !s.isTrustedAssetURL(publicURL) {
		return nil, fmt.Errorf("%w: asset URL is not trusted", ErrInvalidTestimonialImage)
	}
	return &publicURL, nil
}

func (s *testimonialService) isTrustedAssetURL(rawURL string) bool {
	if strings.TrimSpace(rawURL) == "" {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	if s.publicAssetBase == "" {
		return true
	}
	base := strings.TrimRight(strings.ToLower(strings.TrimSpace(s.publicAssetBase)), "/")
	candidate := strings.TrimRight(strings.ToLower(strings.TrimSpace(rawURL)), "/")
	return strings.HasPrefix(candidate, base+"/") || candidate == base
}
