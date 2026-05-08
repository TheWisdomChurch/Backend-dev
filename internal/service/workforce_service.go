package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

type WorkforceService interface {
	List(page, limit int, department, status string) ([]models.WorkforceMember, int64, error)
	Create(req *models.CreateWorkforceRequest) (*models.WorkforceMember, error)
	CreateApplication(req *models.CreateWorkforceRequest) (*models.WorkforceMember, error)
	RegisterExisting(req *models.CreateWorkforceRequest) (*models.WorkforceMember, error)
	Update(id string, req *models.UpdateWorkforceRequest) (*models.WorkforceMember, error)
	Approve(id string) (*models.WorkforceMember, error)
	RequestDelete(id string, requestedBy *models.User) (*models.ApprovalRequest, error)
	ApproveDelete(id string, approver *models.User) error
	Stats() (*models.WorkforceStatsResponse, error)

	// Birthday scheduler helpers
	BirthdayStats() (*models.BirthdayStatsResponse, error)
	BirthdaysByMonth(month int) ([]models.WorkforceMember, error)
	BirthdaysToday(now time.Time) ([]models.WorkforceMember, error)
	SendBirthdayGreetings(month, day int) (*models.BirthdaySendResult, error)
}

type workforceService struct {
	repo        repository.WorkforceRepository
	notifySvc   AdminNotificationService
	approvalSvc ApprovalService
	sender      EmailSender
	branding    email.Branding
}

func NewWorkforceService(
	repo repository.WorkforceRepository,
	notifySvc AdminNotificationService,
	approvalSvc ApprovalService,
	sender EmailSender,
	branding email.Branding,
) WorkforceService {
	return &workforceService{
		repo:        repo,
		notifySvc:   notifySvc,
		approvalSvc: approvalSvc,
		sender:      sender,
		branding:    branding,
	}
}

func (s *workforceService) List(page, limit int, department, status string) ([]models.WorkforceMember, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 300 {
		limit = 10
	}
	offset := (page - 1) * limit
	return s.repo.List(offset, limit, department, status)
}

func (s *workforceService) Create(req *models.CreateWorkforceRequest) (*models.WorkforceMember, error) {
	status := req.Status
	if status == "" {
		status = models.WorkforceStatusPending
	}
	if status == models.WorkforceStatusNew {
		status = models.WorkforceStatusPending
	}
	switch status {
	case models.WorkforceStatusPending, models.WorkforceStatusServing, models.WorkforceStatusNotServing:
	default:
		return nil, errors.New("status must be pending, serving, or not_serving")
	}
	return s.createWithStatus(req, status)
}

func (s *workforceService) CreateApplication(req *models.CreateWorkforceRequest) (*models.WorkforceMember, error) {
	return s.createWithStatus(req, models.WorkforceStatusPending)
}

func (s *workforceService) RegisterExisting(req *models.CreateWorkforceRequest) (*models.WorkforceMember, error) {
	return s.createWithStatus(req, models.WorkforceStatusServing)
}

func (s *workforceService) createWithStatus(req *models.CreateWorkforceRequest, status models.WorkforceStatus) (*models.WorkforceMember, error) {
	if strings.TrimSpace(req.FirstName) == "" || strings.TrimSpace(req.LastName) == "" {
		return nil, errors.New("firstName and lastName are required")
	}
	if strings.TrimSpace(req.Department) == "" {
		return nil, errors.New("department is required")
	}

	month, day, err := parseBirthday(req.BirthdayMonth, req.BirthdayDay, req.Birthday)
	if err != nil {
		return nil, err
	}

	member := &models.WorkforceMember{
		FirstName:     strings.TrimSpace(req.FirstName),
		LastName:      strings.TrimSpace(req.LastName),
		Email:         optionalStringPtr(req.Email),
		Phone:         optionalStringPtr(req.Phone),
		Department:    strings.TrimSpace(req.Department),
		SourceChannel: strings.TrimSpace(req.SourceChannel),
		Status:        status,
		Notes:         req.Notes,
		BirthdayMonth: month,
		BirthdayDay:   day,
	}
	if member.SourceChannel == "" {
		member.SourceChannel = "frontend:web:workforce"
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
	if req.BirthdayMonth != nil || req.BirthdayDay != nil || req.Birthday != nil {
		month, day, err := parseBirthday(req.BirthdayMonth, req.BirthdayDay, req.Birthday)
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

func (s *workforceService) BirthdayStats() (*models.BirthdayStatsResponse, error) {
	counts, total, err := s.repo.BirthdayCountsByMonth(string(models.WorkforceStatusServing))
	if err != nil {
		return nil, err
	}

	months := make([]models.BirthdayMonthCount, 0, 12)
	for m := 1; m <= 12; m++ {
		months = append(months, models.BirthdayMonthCount{
			Month: m,
			Count: counts[m],
		})
	}

	return &models.BirthdayStatsResponse{
		Total:   total,
		ByMonth: months,
	}, nil
}

func (s *workforceService) BirthdaysByMonth(month int) ([]models.WorkforceMember, error) {
	if month < 1 || month > 12 {
		return nil, errors.New("month must be 1-12")
	}
	return s.repo.ListByMonth(month, string(models.WorkforceStatusServing))
}

func (s *workforceService) BirthdaysToday(now time.Time) ([]models.WorkforceMember, error) {
	if now.IsZero() {
		now = time.Now()
	}
	month := int(now.Month())
	day := now.Day()
	return s.repo.ListByMonthDay(month, day, string(models.WorkforceStatusServing))
}

func (s *workforceService) SendBirthdayGreetings(month, day int) (*models.BirthdaySendResult, error) {
	if s.sender == nil {
		return nil, errors.New("email sender is not configured")
	}
	if month < 1 || month > 12 {
		return nil, errors.New("month must be 1-12")
	}
	if day < 1 || day > 31 {
		return nil, errors.New("day must be 1-31")
	}

	members, err := s.repo.ListByMonthDay(month, day, string(models.WorkforceStatusServing))
	if err != nil {
		return nil, err
	}

	appName := strings.TrimSpace(s.branding.AppName)
	if appName == "" {
		appName = "Wisdom House"
	}

	dateLabel := fmt.Sprintf("%02d/%02d", day, month)
	subject := fmt.Sprintf("Happy Birthday from %s", appName)
	heroURL := email.TemplateAssetURL(s.branding, "birthday", "hero.png")

	result := &models.BirthdaySendResult{
		Targeted: len(members),
	}
	var tplStore *email.TemplateStore
	if store, err := email.NewTemplateStoreFromEnv(); err == nil {
		tplStore = store
	}

	for i := range members {
		addr := strings.TrimSpace(ptrString(members[i].Email))
		if addr == "" {
			result.Skipped++
			continue
		}
		fullName := strings.TrimSpace(strings.Join([]string{members[i].FirstName, members[i].LastName}, " "))
		data := email.BirthdayTemplateData{
			Branding:      s.branding,
			RecipientName: fullName,
			BirthdayDate:  dateLabel,
			HeroImageURL:  heroURL,
		}
		body := ""
		if tplStore != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			_, htmlOut, _, err := tplStore.RenderWithData(ctx, "birthday", data)
			cancel()
			if err == nil && strings.TrimSpace(htmlOut) != "" {
				body = htmlOut
			}
		}
		if strings.TrimSpace(body) == "" {
			body = email.RenderBirthdayEmail(data)
		}

		if err := s.sender.SendHTML(addr, subject, body); err != nil {
			result.Skipped++
			continue
		}
		result.Sent++
	}

	return result, nil
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

func (s *workforceService) RequestDelete(id string, requestedBy *models.User) (*models.ApprovalRequest, error) {
	if s.approvalSvc == nil {
		return nil, errors.New("approval service not configured")
	}

	member, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	label := workforceMemberName(member)
	if label == "" {
		label = id
	}

	var requestedByID, requestedByName, requestedByEmail *string
	if requestedBy != nil {
		requestedByID = &requestedBy.ID
		name := strings.TrimSpace(strings.Join([]string{requestedBy.FirstName, requestedBy.LastName}, " "))
		if name != "" {
			requestedByName = &name
		}
		if strings.TrimSpace(requestedBy.Email) != "" {
			email := strings.TrimSpace(requestedBy.Email)
			requestedByEmail = &email
		}
	}

	req, err := s.approvalSvc.CreateRequest(CreateApprovalRequest{
		Type:             models.ApprovalTypeWorkforceDelete,
		EntityID:         &member.ID,
		EntityLabel:      &label,
		RequestedByID:    requestedByID,
		RequestedByName:  requestedByName,
		RequestedByEmail: requestedByEmail,
	})
	if err != nil {
		return nil, err
	}

	s.notifyWorkforceDeleteRequest(member, req)
	return req, nil
}

func (s *workforceService) ApproveDelete(id string, approver *models.User) error {
	if s.approvalSvc == nil {
		return errors.New("approval service not configured")
	}

	entityID := strings.TrimSpace(id)
	if entityID == "" {
		return errors.New("workforce member id or approval request id is required")
	}

	member, err := s.repo.GetByID(entityID)
	var requestID string
	if err != nil {
		req, reqErr := s.approvalSvc.GetRequest(entityID)
		if reqErr != nil {
			if _, completeErr := s.approvalSvc.CompleteRequest(
				models.ApprovalTypeWorkforceDelete,
				entityID,
				models.ApprovalStatusApproved,
				approver,
			); completeErr == nil {
				return nil
			}
			return err
		}
		if req.Type != models.ApprovalTypeWorkforceDelete {
			return errors.New("approval request is not for workforce deletion")
		}
		if req.EntityID == nil || strings.TrimSpace(*req.EntityID) == "" {
			return errors.New("approval request has no workforce member id")
		}
		requestID = req.ID
		entityID = strings.TrimSpace(*req.EntityID)
		member, err = s.repo.GetByID(entityID)
		if err != nil {
			if _, completeErr := s.approvalSvc.CompleteRequestByID(
				requestID,
				models.ApprovalStatusApproved,
				approver,
			); completeErr == nil {
				return nil
			}
			return err
		}
	}

	req, err := s.approvalSvc.CompleteRequest(
		models.ApprovalTypeWorkforceDelete,
		entityID,
		models.ApprovalStatusApproved,
		approver,
	)
	if err != nil {
		return err
	}
	if err := s.repo.Delete(entityID); err != nil {
		return err
	}
	s.notifyWorkforceDeleteApproved(member, req)
	return nil
}

// normalizeBirthday validates optional month/day (1-12, 1-31). Returns nil pointers when absent.

func workforceMemberName(member *models.WorkforceMember) string {
	if member == nil {
		return ""
	}
	return strings.TrimSpace(strings.Join([]string{member.FirstName, member.LastName}, " "))
}

func (s *workforceService) notifyWorkforceDeleteRequest(member *models.WorkforceMember, req *models.ApprovalRequest) {
	if s.notifySvc == nil || member == nil || req == nil {
		return
	}
	fullName := workforceMemberName(member)
	if fullName == "" {
		fullName = "Workforce profile"
	}
	title := "Workforce delete approval required"
	message := fmt.Sprintf("%s was marked for deletion from %s. Super admin approval is required before removal.", fullName, member.Department)
	entityType := "workforce_delete"
	entityID := member.ID
	_ = s.notifySvc.NotifyRoles(AdminNotificationInput{
		Type:       "workforce_delete_request",
		Title:      title,
		Message:    message,
		TicketCode: &req.TicketCode,
		EntityType: &entityType,
		EntityID:   &entityID,
		Roles:      []string{"super_admin"},
	})
}

func (s *workforceService) notifyWorkforceDeleteApproved(member *models.WorkforceMember, req *models.ApprovalRequest) {
	if s.notifySvc == nil || member == nil {
		return
	}
	fullName := workforceMemberName(member)
	if fullName == "" {
		fullName = "Workforce profile"
	}
	title := "Workforce delete approved"
	message := fmt.Sprintf("%s has been approved and removed from workforce records.", fullName)
	entityType := "workforce_delete"
	entityID := member.ID
	var ticket *string
	if req != nil {
		ticket = &req.TicketCode
	}
	_ = s.notifySvc.NotifyRoles(AdminNotificationInput{
		Type:       "workforce_delete_approved",
		Title:      title,
		Message:    message,
		TicketCode: ticket,
		EntityType: &entityType,
		EntityID:   &entityID,
		Roles:      []string{"admin"},
	})
}

func (s *workforceService) sendApprovalEmail(member *models.WorkforceMember) {
	addr := ""
	if member != nil {
		addr = strings.TrimSpace(ptrString(member.Email))
	}
	if s.sender == nil || member == nil || addr == "" {
		return
	}

	fullName := strings.TrimSpace(strings.Join([]string{member.FirstName, member.LastName}, " "))
	body := email.RenderWorkforceApprovalEmail(email.WorkforceApprovalTemplateData{
		Branding:      s.branding,
		RecipientName: fullName,
		Department:    member.Department,
	})
	subject := "Welcome to the workforce"
	_ = s.sender.SendHTML(addr, subject, body)
}
