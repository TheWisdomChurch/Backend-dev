package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
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
	repo        repository.TestimonialRepository
	uploader    AssetUploader
	approvalSvc ApprovalService
	notifySvc   AdminNotificationService
}

func NewTestimonialService(
	repo repository.TestimonialRepository,
	uploader AssetUploader,
	approvalSvc ApprovalService,
	notifySvc AdminNotificationService,
) TestimonialService {
	return &testimonialService{
		repo:        repo,
		uploader:    uploader,
		approvalSvc: approvalSvc,
		notifySvc:   notifySvc,
	}
}

func (s *testimonialService) CreateTestimonial(req *models.CreateTestimonialRequest) (*models.Testimonial, error) {
	imageURL, err := s.maybeUploadImage(req.ImageURL)
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
				Roles:      []string{"admin", "super_admin"},
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
		imageURL, err := s.maybeUploadImage(req.ImageURL)
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

// maybeUploadImage uploads a base64/dataURL image to Spaces and returns the public URL pointer.
// If input is nil or already a URL (http/https), it is returned as-is.
func (s *testimonialService) maybeUploadImage(image *string) (*string, error) {
	if image == nil || s.uploader == nil {
		return image, nil
	}

	raw := strings.TrimSpace(*image)
	if raw == "" {
		return nil, nil
	}
	low := strings.ToLower(raw)
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		return &raw, nil
	}

	// Expect data URL: data:image/png;base64,....
	var mimeType string
	var b64data string

	if strings.HasPrefix(low, "data:") {
		parts := strings.SplitN(raw, ",", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid data URL for image")
		}
		meta := parts[0]
		b64data = parts[1]

		// meta like "data:image/png;base64"
		if strings.Contains(meta, ";") {
			mimeType = strings.TrimPrefix(strings.Split(meta, ";")[0], "data:")
		}
	} else {
		// assume plain base64, default to png
		mimeType = "image/png"
		b64data = raw
	}

	if mimeType == "" {
		mimeType = "image/png"
	}

	decoded, err := base64.StdEncoding.DecodeString(b64data)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 image: %w", err)
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

	url, err := s.uploader.Upload(ctx, key, mimeType, bytes.NewReader(decoded))
	if err != nil {
		return nil, err
	}

	return &url, nil
}
