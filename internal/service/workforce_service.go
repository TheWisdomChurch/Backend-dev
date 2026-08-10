package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
	"wisdomHouse-backend/internal/sanitize"
)

type WorkforceService interface {
	List(page, limit int, department, status string) ([]models.WorkforceMember, int64, error)
	Create(req *models.CreateWorkforceRequest) (*models.WorkforceMember, error)
	CreateApplication(req *models.CreateWorkforceRequest) (*models.WorkforceMember, error)
	LookupByEmail(email string) (*models.WorkforceMember, error)
	RegisterExisting(req *models.CreateWorkforceRequest) (*models.WorkforceMember, error)
	Update(id string, req *models.UpdateWorkforceRequest) (*models.WorkforceMember, error)
	Approve(id string) (*models.WorkforceMember, error)
	RejectRegistration(id, reason string, approver *models.User) (*models.WorkforceMember, error)
	RequestDelete(id, reason string, requestedBy *models.User) (*models.ApprovalRequest, error)
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
	member, err := s.createWithStatus(req, models.WorkforceStatusPending)
	if err != nil {
		return nil, err
	}
	s.createRegistrationTicket(member)
	return member, nil
}

// createRegistrationTicket raises a super-admin ticket for a new pending
// workforce application. Without this, "pending" was just a status field any
// admin could flip via the regular edit form — there was no queue a super
// admin could actually discover the application in, even though the real
// approve action (below) was already super-admin-gated.
func (s *workforceService) createRegistrationTicket(member *models.WorkforceMember) {
	if s.approvalSvc == nil || member == nil {
		return
	}
	label := workforceMemberName(member)
	if label == "" {
		label = member.ID
	}
	req, err := s.approvalSvc.CreateRequest(CreateApprovalRequest{
		Type:        models.ApprovalTypeWorkforceRegistration,
		EntityID:    &member.ID,
		EntityLabel: &label,
	})
	if err != nil {
		slog.Warn("workforce_service: failed to create registration approval ticket", "member_id", member.ID, "error", err)
		return
	}
	s.notifyWorkforceRegistration(member, req)
}

func (s *workforceService) notifyWorkforceRegistration(member *models.WorkforceMember, req *models.ApprovalRequest) {
	if s.notifySvc == nil || member == nil || req == nil {
		return
	}
	fullName := workforceMemberName(member)
	if fullName == "" {
		fullName = "A new applicant"
	}
	title := "New workforce registration"
	message := fmt.Sprintf("%s applied to join the workforce in %s and is awaiting super admin approval.", fullName, member.Department)
	entityType := "workforce_registration"
	entityID := member.ID
	if err := s.notifySvc.NotifyRoles(AdminNotificationInput{
		Type:       "workforce_registration_request",
		Title:      title,
		Message:    message,
		TicketCode: &req.TicketCode,
		EntityType: &entityType,
		EntityID:   &entityID,
		Roles:      []string{"super_admin"},
	}); err != nil {
		slog.Warn("workforce_service: failed to notify super admins of registration", "member_id", member.ID, "error", err)
	}
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

	anniversaryMonth, anniversaryDay, err := parseAnniversary(req.AnniversaryMonth, req.AnniversaryDay, req.Anniversary)
	if err != nil {
		return nil, err
	}

	var maritalStatus *string
	switch strings.ToLower(strings.TrimSpace(req.Married)) {
	case "yes":
		maritalStatus = strPtr("married")
	case "no":
		maritalStatus = strPtr("single")
	}

	member := &models.WorkforceMember{
		FirstName:        strings.TrimSpace(req.FirstName),
		LastName:         strings.TrimSpace(req.LastName),
		Email:            optionalStringPtr(req.Email),
		Phone:            optionalStringPtr(req.Phone),
		Department:       strings.TrimSpace(req.Department),
		SourceChannel:    strings.TrimSpace(req.SourceChannel),
		Status:           status,
		Notes:            sanitize.TextPtr(req.Notes),
		BirthdayMonth:    month,
		BirthdayDay:      day,
		Occupation:       optionalStringPtr(req.Occupation),
		MaritalStatus:    maritalStatus,
		SpouseName:       optionalStringPtr(req.Spouse),
		AnniversaryMonth: anniversaryMonth,
		AnniversaryDay:   anniversaryDay,
		About:            sanitize.TextPtr(optionalStringPtr(req.About)),
	}
	if member.SourceChannel == "" {
		member.SourceChannel = "frontend:web:workforce"
	}

	if err := s.repo.Create(member); err != nil {
		return nil, err
	}
	return member, nil
}

func (s *workforceService) LookupByEmail(email string) (*models.WorkforceMember, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil, errors.New("email is required")
	}
	return s.repo.FindByEmail(email)
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
		// Moving a member OUT of pending is what Approve/RejectRegistration
		// are for — they also resolve the registration ticket. Letting this
		// generic edit form silently flip the same field would leave that
		// ticket open forever while the member already shows as decided.
		current, err := s.repo.GetByID(id)
		if err != nil {
			return nil, err
		}
		if (current.Status == models.WorkforceStatusPending || current.Status == models.WorkforceStatusNew) && *req.Status != current.Status {
			return nil, errors.New("use the approve or reject action to decide a pending registration")
		}
		updates["status"] = *req.Status
	}
	if req.Notes != nil {
		updates["notes"] = sanitize.TextPtr(req.Notes)
	}
	if req.BirthdayMonth != nil || req.BirthdayDay != nil || req.Birthday != nil {
		month, day, err := parseBirthday(req.BirthdayMonth, req.BirthdayDay, req.Birthday)
		if err != nil {
			return nil, err
		}
		updates["birthday_month"] = month
		updates["birthday_day"] = day
	}
	if req.Occupation != nil {
		updates["occupation"] = strings.TrimSpace(*req.Occupation)
	}
	if req.Married != nil {
		switch strings.ToLower(strings.TrimSpace(*req.Married)) {
		case "yes":
			updates["marital_status"] = "married"
		case "no":
			updates["marital_status"] = "single"
		}
	}
	if req.Spouse != nil {
		updates["spouse_name"] = strings.TrimSpace(*req.Spouse)
	}
	if req.About != nil {
		updates["about"] = sanitize.Text(*req.About)
	}
	if req.AnniversaryMonth != nil || req.AnniversaryDay != nil || req.Anniversary != nil {
		month, day, err := parseAnniversary(req.AnniversaryMonth, req.AnniversaryDay, req.Anniversary)
		if err != nil {
			return nil, err
		}
		updates["anniversary_month"] = month
		updates["anniversary_day"] = day
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

	if s.approvalSvc != nil {
		if _, err := s.approvalSvc.CompleteRequest(models.ApprovalTypeWorkforceRegistration, updated.ID, models.ApprovalStatusApproved, nil); err != nil {
			slog.Warn("workforce_service: failed to complete registration ticket on approve", "member_id", updated.ID, "error", err)
		}
	}

	s.sendApprovalEmail(updated)
	return updated, nil
}

// RejectRegistration declines a pending workforce application. The member
// row is kept (as not_serving) rather than deleted, mirroring how admin
// user rejection deactivates rather than removes — reversible, with an
// audit trail, instead of silently vanishing.
func (s *workforceService) RejectRegistration(id, reason string, approver *models.User) (*models.WorkforceMember, error) {
	member, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if member.Status != models.WorkforceStatusPending && member.Status != models.WorkforceStatusNew {
		return nil, errors.New("member is not awaiting approval")
	}

	updated, err := s.repo.Update(id, map[string]interface{}{"status": models.WorkforceStatusNotServing})
	if err != nil {
		return nil, err
	}

	if s.approvalSvc != nil {
		if _, err := s.approvalSvc.CompleteRequest(models.ApprovalTypeWorkforceRegistration, updated.ID, models.ApprovalStatusRejected, approver); err != nil {
			slog.Warn("workforce_service: failed to complete registration ticket on reject", "member_id", updated.ID, "error", err)
		}
	}

	s.sendRejectionEmail(updated, reason)
	return updated, nil
}

func (s *workforceService) RequestDelete(id, reason string, requestedBy *models.User) (*models.ApprovalRequest, error) {
	if s.approvalSvc == nil {
		return nil, errors.New("approval service not configured")
	}

	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, errors.New("a reason is required")
	}

	member, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	label := workforceMemberName(member)
	if label == "" {
		label = id
	}

	requestedByID, requestedByName, requestedByEmail := requestedBy.ApprovalRequesterFields()

	req, err := s.approvalSvc.CreateRequest(CreateApprovalRequest{
		Type:             models.ApprovalTypeWorkforceDelete,
		EntityID:         &member.ID,
		EntityLabel:      &label,
		Reason:           &reason,
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
	if err := s.notifySvc.NotifyRoles(AdminNotificationInput{
		Type:       "workforce_delete_request",
		Title:      title,
		Message:    message,
		TicketCode: &req.TicketCode,
		EntityType: &entityType,
		EntityID:   &entityID,
		Roles:      []string{"super_admin"},
	}); err != nil {
		slog.Warn("workforce_service: failed to send delete-request notification", "member_id", member.ID, "error", err)
	}
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
	if err := s.notifySvc.NotifyRoles(AdminNotificationInput{
		Type:       "workforce_delete_approved",
		Title:      title,
		Message:    message,
		TicketCode: ticket,
		EntityType: &entityType,
		EntityID:   &entityID,
		Roles:      []string{"admin"},
	}); err != nil {
		slog.Warn("workforce_service: failed to send delete-approved notification", "member_id", member.ID, "error", err)
	}
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
	if err := s.sender.SendHTML(addr, subject, body); err != nil {
		slog.Warn("workforce_service: failed to send approval email", "member_id", member.ID, "error", err)
	}
}

func (s *workforceService) sendRejectionEmail(member *models.WorkforceMember, reason string) {
	addr := ""
	if member != nil {
		addr = strings.TrimSpace(ptrString(member.Email))
	}
	if s.sender == nil || member == nil || addr == "" {
		return
	}

	appName := strings.TrimSpace(s.branding.AppName)
	if appName == "" {
		appName = "Wisdom House"
	}
	fullName := strings.TrimSpace(strings.Join([]string{member.FirstName, member.LastName}, " "))
	if fullName == "" {
		fullName = "there"
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "Your application was not approved at this time."
	}

	body := fmt.Sprintf(`<!doctype html>
<html>
  <body style="margin:0;padding:0;background:#f8fafc;font-family:Arial,sans-serif;color:#111827;">
    <table width="100%%" cellpadding="0" cellspacing="0" style="padding:28px 14px;">
      <tr>
        <td align="center">
          <table width="100%%" style="max-width:520px;background:#ffffff;border:1px solid #e5e7eb;border-radius:12px;padding:28px;">
            <tr><td>
              <p style="margin:0 0 12px;font-size:16px;">Hi %s,</p>
              <p style="margin:0 0 12px;font-size:15px;line-height:1.6;">Thank you for applying to join the workforce at %s. After review, we're not able to move forward with your application right now.</p>
              <p style="margin:0 0 12px;font-size:15px;line-height:1.6;"><strong>Reason:</strong> %s</p>
              <p style="margin:0;font-size:15px;line-height:1.6;">You're welcome to apply again in the future.</p>
            </td></tr>
          </table>
        </td>
      </tr>
    </table>
  </body>
</html>`, fullName, appName, reason)

	subject := "Update on your workforce application"
	if err := s.sender.SendHTML(addr, subject, body); err != nil {
		slog.Warn("workforce_service: failed to send rejection email", "member_id", member.ID, "error", err)
	}
}
