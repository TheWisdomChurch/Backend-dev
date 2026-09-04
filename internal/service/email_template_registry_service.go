package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"html/template"
	"strings"

	"gorm.io/datatypes"

	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

type EmailTemplateRegistryService interface {
	Create(req *models.CreateEmailTemplateRequest, createdBy *string) (*models.EmailTemplate, error)
	Update(id string, req *models.UpdateEmailTemplateRequest) (*models.EmailTemplate, error)
	GetByID(id string) (*models.EmailTemplate, error)
	List(page, limit int, ownerType, ownerID, templateKey, status string) ([]models.EmailTemplate, int64, error)
	Activate(id string) (*models.EmailTemplate, error)
	RenderHTML(tpl *models.EmailTemplate, data any) (string, error)
	// Preview renders content through the exact same code path Create/Update
	// use to bake HTMLBody, without persisting anything — this is what the
	// admin portal's live preview calls, so preview and saved output can
	// never drift from each other.
	Preview(content models.FormEmailContent) (htmlBody string, textBody string)
}

type emailTemplateRegistryService struct {
	repo     repository.EmailTemplateRepository
	branding email.Branding
}

func NewEmailTemplateRegistryService(repo repository.EmailTemplateRepository, branding email.Branding) EmailTemplateRegistryService {
	return &emailTemplateRegistryService{repo: repo, branding: branding}
}

// applyContent renders req content via the shared theme and writes the
// result into tpl.HTMLBody/TextBody/ContentJSON. It's the only path by which
// HTMLBody is set for a content-driven template — callers never accept a
// client-supplied HTMLBody alongside Content.
func (s *emailTemplateRegistryService) applyContent(tpl *models.EmailTemplate, content models.FormEmailContent) error {
	htmlBody, textBody := email.RenderFormEmailContent(s.branding, content)

	contentBytes, err := json.Marshal(content)
	if err != nil {
		return err
	}

	tpl.HTMLBody = htmlBody
	tpl.TextBody = &textBody
	tpl.ContentJSON = datatypes.JSON(contentBytes)
	return nil
}

func (s *emailTemplateRegistryService) Preview(content models.FormEmailContent) (string, string) {
	return email.RenderFormEmailContent(s.branding, content)
}

func (s *emailTemplateRegistryService) Create(req *models.CreateEmailTemplateRequest, createdBy *string) (*models.EmailTemplate, error) {
	if req == nil {
		return nil, errors.New("request is required")
	}
	key := strings.TrimSpace(req.TemplateKey)
	if key == "" {
		return nil, errors.New("templateKey is required")
	}
	if strings.Contains(key, "..") {
		return nil, errors.New("templateKey contains invalid characters")
	}

	ownerType := cleanLowerPtr(req.OwnerType)
	ownerID := cleanPtr(req.OwnerID)
	if ownerType != nil && ownerID == nil {
		return nil, errors.New("ownerId is required when ownerType is set")
	}

	status := models.EmailTemplateDraft
	if req.Status != nil {
		status = *req.Status
	}

	version := 1
	if req.Version != nil && *req.Version > 0 {
		version = *req.Version
	} else {
		next, err := s.repo.NextVersion(key)
		if err != nil {
			return nil, err
		}
		version = next
	}

	tpl := &models.EmailTemplate{
		TemplateKey: key,
		OwnerType:   ownerType,
		OwnerID:     ownerID,
		Subject:     cleanPtr(req.Subject),
		HTMLBody:    strings.TrimSpace(req.HTMLBody),
		TextBody:    cleanPtr(req.TextBody),
		Status:      status,
		Version:     version,
		IsActive:    req.Activate,
		CreatedByID: createdBy,
	}

	if req.Content != nil {
		if err := s.applyContent(tpl, *req.Content); err != nil {
			return nil, err
		}
	}
	if tpl.HTMLBody == "" {
		return nil, errors.New("htmlBody or content is required")
	}

	if req.Activate {
		if err := s.repo.ActivateExclusive(tpl); err != nil {
			return nil, err
		}
	} else if err := s.repo.Create(tpl); err != nil {
		return nil, err
	}

	return tpl, nil
}

func (s *emailTemplateRegistryService) Update(id string, req *models.UpdateEmailTemplateRequest) (*models.EmailTemplate, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("id is required")
	}
	if req == nil {
		return nil, errors.New("request is required")
	}

	tpl, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	if req.TemplateKey != nil {
		key := strings.TrimSpace(*req.TemplateKey)
		if key == "" {
			return nil, errors.New("templateKey cannot be empty")
		}
		if strings.Contains(key, "..") {
			return nil, errors.New("templateKey contains invalid characters")
		}
		tpl.TemplateKey = key
	}
	if req.OwnerType != nil {
		ownerType := strings.ToLower(strings.TrimSpace(*req.OwnerType))
		if ownerType == "" {
			tpl.OwnerType = nil
			tpl.OwnerID = nil
		} else {
			tpl.OwnerType = &ownerType
		}
	}
	if req.OwnerID != nil {
		ownerID := strings.TrimSpace(*req.OwnerID)
		if ownerID == "" {
			tpl.OwnerID = nil
		} else {
			tpl.OwnerID = &ownerID
		}
	}
	if req.Subject != nil {
		tpl.Subject = cleanPtr(req.Subject)
	}
	if req.Content != nil {
		// Content-driven templates are re-rendered from scratch, so a
		// directly-supplied HTMLBody/TextBody alongside Content would be
		// silently discarded and misleading — reject it instead.
		if req.HTMLBody != nil || req.TextBody != nil {
			return nil, errors.New("htmlBody/textBody cannot be set together with content")
		}
		if err := s.applyContent(tpl, *req.Content); err != nil {
			return nil, err
		}
	} else {
		if req.HTMLBody != nil {
			body := strings.TrimSpace(*req.HTMLBody)
			if body == "" {
				return nil, errors.New("htmlBody cannot be empty")
			}
			tpl.HTMLBody = body
			// Switching to a hand-authored body: it no longer reflects
			// structured content, so drop the stale snapshot.
			tpl.ContentJSON = nil
		}
		if req.TextBody != nil {
			tpl.TextBody = cleanPtr(req.TextBody)
		}
	}
	if req.Status != nil {
		tpl.Status = *req.Status
	}
	if req.Version != nil && *req.Version > 0 {
		tpl.Version = *req.Version
	}

	if req.Activate != nil && *req.Activate {
		if err := s.repo.ActivateExclusive(tpl); err != nil {
			return nil, err
		}
		return tpl, nil
	}

	if err := s.repo.Update(tpl); err != nil {
		return nil, err
	}

	return tpl, nil
}

func (s *emailTemplateRegistryService) GetByID(id string) (*models.EmailTemplate, error) {
	return s.repo.GetByID(id)
}

func (s *emailTemplateRegistryService) List(page, limit int, ownerType, ownerID, templateKey, status string) ([]models.EmailTemplate, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}
	offset := (page - 1) * limit
	return s.repo.List(offset, limit, ownerType, ownerID, templateKey, status)
}

func (s *emailTemplateRegistryService) Activate(id string) (*models.EmailTemplate, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("id is required")
	}
	tpl, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	if err := s.repo.ActivateExclusive(tpl); err != nil {
		return nil, err
	}
	return tpl, nil
}

func (s *emailTemplateRegistryService) RenderHTML(tpl *models.EmailTemplate, data any) (string, error) {
	if tpl == nil {
		return "", errors.New("template is required")
	}
	raw := strings.TrimSpace(tpl.HTMLBody)
	if raw == "" {
		return "", errors.New("htmlBody is empty")
	}

	t, err := template.New("db").Option("missingkey=error").Parse(raw)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func cleanPtr(v *string) *string {
	if v == nil {
		return nil
	}
	s := strings.TrimSpace(*v)
	if s == "" {
		return nil
	}
	return &s
}

func cleanLowerPtr(v *string) *string {
	if v == nil {
		return nil
	}
	s := strings.ToLower(strings.TrimSpace(*v))
	if s == "" {
		return nil
	}
	return &s
}
