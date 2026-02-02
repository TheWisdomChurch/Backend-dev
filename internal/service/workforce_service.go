package service

import (
	"errors"
	"strings"

	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

type WorkforceService interface {
	List(page, limit int, department, status string) ([]models.WorkforceMember, int64, error)
	Create(req *models.CreateWorkforceRequest) (*models.WorkforceMember, error)
	Update(id string, req *models.UpdateWorkforceRequest) (*models.WorkforceMember, error)
	Approve(id string) (*models.WorkforceMember, error)
	Stats() (*models.WorkforceStatsResponse, error)
}

type workforceService struct {
	repo     repository.WorkforceRepository
	sender   EmailSender
	branding email.Branding
}

func NewWorkforceService(repo repository.WorkforceRepository, sender EmailSender, branding email.Branding) WorkforceService {
	return &workforceService{repo: repo, sender: sender, branding: branding}
}

func (s *workforceService) List(page, limit int, department, status string) ([]models.WorkforceMember, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}
	offset := (page - 1) * limit
	return s.repo.List(offset, limit, department, status)
}

func (s *workforceService) Create(req *models.CreateWorkforceRequest) (*models.WorkforceMember, error) {
	if strings.TrimSpace(req.FirstName) == "" || strings.TrimSpace(req.LastName) == "" {
		return nil, errors.New("firstName and lastName are required")
	}
	if strings.TrimSpace(req.Department) == "" {
		return nil, errors.New("department is required")
	}

	month, day, err := normalizeBirthday(req.BirthdayMonth, req.BirthdayDay)
	if err != nil {
		return nil, err
	}

	if req.Status != "" && req.Status != models.WorkforceStatusPending && req.Status != models.WorkforceStatusNew {
		return nil, errors.New("new workforce requests must start as pending")
	}
	status := models.WorkforceStatusPending

	member := &models.WorkforceMember{
		FirstName:     strings.TrimSpace(req.FirstName),
		LastName:      strings.TrimSpace(req.LastName),
		Email:         strings.TrimSpace(req.Email),
		Phone:         strings.TrimSpace(req.Phone),
		Department:    strings.TrimSpace(req.Department),
		Status:        status,
		Notes:         req.Notes,
		BirthdayMonth: month,
		BirthdayDay:   day,
	}

	if err := s.repo.Create(member); err != nil {
		return nil, err
	}
	return member, nil
}

func (s *workforceService) Update(id string, req *models.UpdateWorkforceRequest) (*models.WorkforceMember, error) {
	updates := map[string]interface{}{}
	if req.FirstName != nil {
		updates["first_name"] = strings.TrimSpace(*req.FirstName)
	}
	if req.LastName != nil {
		updates["last_name"] = strings.TrimSpace(*req.LastName)
	}
	if req.Email != nil {
		updates["email"] = strings.TrimSpace(*req.Email)
	}
	if req.Phone != nil {
		updates["phone"] = strings.TrimSpace(*req.Phone)
	}
	if req.Department != nil {
		updates["department"] = strings.TrimSpace(*req.Department)
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if req.Notes != nil {
		updates["notes"] = req.Notes
	}
	if req.BirthdayMonth != nil || req.BirthdayDay != nil {
		month, day, err := normalizeBirthday(req.BirthdayMonth, req.BirthdayDay)
		if err != nil {
			return nil, err
		}
		updates["birthday_month"] = month
		updates["birthday_day"] = day
	}

	if len(updates) == 0 {
		return nil, errors.New("no updates provided")
	}

	return s.repo.Update(id, updates)
}

func (s *workforceService) Stats() (*models.WorkforceStatsResponse, error) {
	return s.repo.Stats()
}

func (s *workforceService) Approve(id string) (*models.WorkforceMember, error) {
	member, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	if member.Status == models.WorkforceStatusServing {
		return member, nil
	}

	if member.Status != models.WorkforceStatusPending && member.Status != models.WorkforceStatusNew {
		return nil, errors.New("member is not awaiting approval")
	}

	member.Status = models.WorkforceStatusServing
	updated, err := s.repo.Update(id, map[string]interface{}{"status": member.Status})
	if err != nil {
		return nil, err
	}

	s.sendApprovalEmail(updated)
	return updated, nil
}

// normalizeBirthday validates optional month/day (1-12, 1-31). Returns nil pointers when absent.
func normalizeBirthday(monthPtr, dayPtr *int) (*int, *int, error) {
	if monthPtr == nil && dayPtr == nil {
		return nil, nil, nil
	}
	if monthPtr == nil || dayPtr == nil {
		return nil, nil, errors.New("birthdayMonth and birthdayDay must both be provided")
	}
	m := *monthPtr
	d := *dayPtr
	if m < 1 || m > 12 {
		return nil, nil, errors.New("birthdayMonth must be 1-12")
	}
	if d < 1 || d > 31 {
		return nil, nil, errors.New("birthdayDay must be 1-31")
	}
	return &m, &d, nil
}

func (s *workforceService) sendApprovalEmail(member *models.WorkforceMember) {
	if s.sender == nil || member == nil || strings.TrimSpace(member.Email) == "" {
		return
	}

	fullName := strings.TrimSpace(strings.Join([]string{member.FirstName, member.LastName}, " "))
	body := email.RenderWorkforceApprovalEmail(email.WorkforceApprovalTemplateData{
		Branding:      s.branding,
		RecipientName: fullName,
		Department:    member.Department,
	})
	subject := "Welcome to the workforce"
	_ = s.sender.SendHTML(member.Email, subject, body)
}
