package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"

	"gorm.io/datatypes"
)

type LeadershipService interface {
	List(page, limit int, role, status string) ([]models.LeadershipMember, int64, error)
	ListApproved(role string) ([]models.LeadershipMember, error)
	Apply(req *models.CreateLeadershipRequest) (*models.LeadershipMember, error)
	Create(req *models.CreateLeadershipRequest) (*models.LeadershipMember, error)
	Update(id string, req *models.UpdateLeadershipRequest) (*models.LeadershipMember, error)
	Approve(id string) (*models.LeadershipMember, error)
	Decline(id string) (*models.LeadershipMember, error)
	RequestDelete(id string, requestedBy *models.User) (*models.ApprovalRequest, error)
	ApproveDelete(id string, approver *models.User) error
	Delete(id string) error

	BirthdayStats() (*models.BirthdayStatsResponse, error)
	BirthdaysByMonth(month int) ([]models.LeadershipMember, error)
	BirthdaysToday(now time.Time) ([]models.LeadershipMember, error)
	SendBirthdayGreetings(month, day int) (*models.BirthdaySendResult, error)

	AnniversaryStats() (*models.BirthdayStatsResponse, error)
	AnniversariesByMonth(month int) ([]models.LeadershipMember, error)
	AnniversariesToday(now time.Time) ([]models.LeadershipMember, error)
	SendAnniversaryGreetings(month, day int) (*models.BirthdaySendResult, error)
}

type leadershipService struct {
	repo        repository.LeadershipRepository
	formRepo    repository.FormRepository
	notifySvc   AdminNotificationService
	approvalSvc ApprovalService
	sender      EmailSender
	branding    email.Branding
}

func NewLeadershipService(
	repo repository.LeadershipRepository,
	formRepo repository.FormRepository,
	notifySvc AdminNotificationService,
	approvalSvc ApprovalService,
	sender EmailSender,
	branding email.Branding,
) LeadershipService {
	return &leadershipService{
		repo:        repo,
		formRepo:    formRepo,
		notifySvc:   notifySvc,
		approvalSvc: approvalSvc,
		sender:      sender,
		branding:    branding,
	}
}

func (s *leadershipService) List(page, limit int, role, status string) ([]models.LeadershipMember, int64, error) {
	s.syncLeadershipIntakeSubmissions()
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 300 {
		limit = 10
	}
	offset := (page - 1) * limit
	return s.repo.List(offset, limit, role, status)
}

func (s *leadershipService) ListApproved(role string) ([]models.LeadershipMember, error) {
	s.syncLeadershipIntakeSubmissions()
	return s.repo.ListApproved(role)
}

func (s *leadershipService) Apply(req *models.CreateLeadershipRequest) (*models.LeadershipMember, error) {
	member, err := s.createWithStatus(req, models.LeadershipStatusPending)
	if err != nil {
		return nil, err
	}
	s.notifyNewApplication(member)
	return member, nil
}

func (s *leadershipService) Create(req *models.CreateLeadershipRequest) (*models.LeadershipMember, error) {
	status := req.Status
	if status == "" {
		status = models.LeadershipStatusPending
	}
	return s.createWithStatus(req, status)
}

func (s *leadershipService) Update(id string, req *models.UpdateLeadershipRequest) (*models.LeadershipMember, error) {
	updates := map[string]interface{}{}
	var changedStatus *models.LeadershipStatus

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
	if req.Role != nil {
		if !isValidLeadershipRole(*req.Role) {
			return nil, errors.New("invalid leadership role")
		}
		updates["role"] = *req.Role
	}
	if req.Status != nil {
		if !isValidLeadershipStatus(*req.Status) {
			return nil, errors.New("invalid leadership status")
		}
		updates["status"] = *req.Status
		changedStatus = req.Status
	}
	if req.Bio != nil {
		updates["bio"] = req.Bio
	}
	if req.ImageURL != nil {
		updates["image_url"] = req.ImageURL
	}
	if req.BirthdayMonth != nil || req.BirthdayDay != nil || req.Birthday != nil {
		month, day, err := parseBirthday(req.BirthdayMonth, req.BirthdayDay, req.Birthday)
		if err != nil {
			return nil, err
		}
		updates["birthday_month"] = month
		updates["birthday_day"] = day
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

	member, err := s.repo.Update(id, updates)
	if err != nil {
		return nil, err
	}

	if changedStatus != nil {
		s.sendLeadershipStatusEmail(member, *changedStatus)
	}
	return member, nil
}

func (s *leadershipService) Approve(id string) (*models.LeadershipMember, error) {
	member, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if member.Status == models.LeadershipStatusApproved {
		return member, nil
	}
	updated, err := s.repo.Update(id, map[string]interface{}{"status": models.LeadershipStatusApproved})
	if err != nil {
		return nil, err
	}
	s.sendLeadershipStatusEmail(updated, models.LeadershipStatusApproved)
	return updated, nil
}

func (s *leadershipService) Decline(id string) (*models.LeadershipMember, error) {
	member, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if member.Status == models.LeadershipStatusDeclined {
		return member, nil
	}
	updated, err := s.repo.Update(id, map[string]interface{}{"status": models.LeadershipStatusDeclined})
	if err != nil {
		return nil, err
	}
	s.sendLeadershipStatusEmail(updated, models.LeadershipStatusDeclined)
	return updated, nil
}

func (s *leadershipService) RequestDelete(id string, requestedBy *models.User) (*models.ApprovalRequest, error) {
	if s.approvalSvc == nil {
		return nil, errors.New("approval service not configured")
	}

	member, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	label := strings.TrimSpace(strings.Join([]string{member.FirstName, member.LastName}, " "))
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
		Type:             models.ApprovalTypeLeadershipDelete,
		EntityID:         &member.ID,
		EntityLabel:      &label,
		RequestedByID:    requestedByID,
		RequestedByName:  requestedByName,
		RequestedByEmail: requestedByEmail,
	})
	if err != nil {
		return nil, err
	}

	s.notifyLeadershipDeleteRequest(member, req)
	return req, nil
}

func (s *leadershipService) ApproveDelete(id string, approver *models.User) error {
	if s.approvalSvc == nil {
		return errors.New("approval service not configured")
	}

	entityID := strings.TrimSpace(id)
	if entityID == "" {
		return errors.New("leadership member id or approval request id is required")
	}

	if _, err := s.repo.GetByID(entityID); err != nil {
		req, reqErr := s.approvalSvc.GetRequest(entityID)
		if reqErr != nil {
			if _, completeErr := s.approvalSvc.CompleteRequest(
				models.ApprovalTypeLeadershipDelete,
				entityID,
				models.ApprovalStatusApproved,
				approver,
			); completeErr == nil {
				return nil
			}
			return err
		}
		if req.Type != models.ApprovalTypeLeadershipDelete {
			return errors.New("approval request is not for leadership deletion")
		}
		if req.EntityID == nil || strings.TrimSpace(*req.EntityID) == "" {
			return errors.New("approval request has no leadership member id")
		}
		entityID = strings.TrimSpace(*req.EntityID)
		if _, err := s.repo.GetByID(entityID); err != nil {
			if _, completeErr := s.approvalSvc.CompleteRequestByID(
				req.ID,
				models.ApprovalStatusApproved,
				approver,
			); completeErr == nil {
				return nil
			}
			return err
		}
	}

	if _, err := s.approvalSvc.CompleteRequest(
		models.ApprovalTypeLeadershipDelete,
		entityID,
		models.ApprovalStatusApproved,
		approver,
	); err != nil {
		return err
	}
	return s.repo.Delete(entityID)
}

func (s *leadershipService) Delete(id string) error {
	return s.repo.Delete(id)
}

func (s *leadershipService) BirthdayStats() (*models.BirthdayStatsResponse, error) {
	counts, total, err := s.repo.BirthdayCountsByMonth(string(models.LeadershipStatusApproved))
	if err != nil {
		return nil, err
	}
	months := make([]models.BirthdayMonthCount, 0, 12)
	for m := 1; m <= 12; m++ {
		months = append(months, models.BirthdayMonthCount{Month: m, Count: counts[m]})
	}
	return &models.BirthdayStatsResponse{
		Total:   total,
		ByMonth: months,
	}, nil
}

func (s *leadershipService) BirthdaysByMonth(month int) ([]models.LeadershipMember, error) {
	if month < 1 || month > 12 {
		return nil, errors.New("month must be 1-12")
	}
	return s.repo.ListByBirthdayMonth(month, string(models.LeadershipStatusApproved))
}

func (s *leadershipService) BirthdaysToday(now time.Time) ([]models.LeadershipMember, error) {
	if now.IsZero() {
		now = time.Now()
	}
	return s.repo.ListByBirthdayMonthDay(int(now.Month()), now.Day(), string(models.LeadershipStatusApproved))
}

func (s *leadershipService) SendBirthdayGreetings(month, day int) (*models.BirthdaySendResult, error) {
	if s.sender == nil {
		return nil, errors.New("email sender is not configured")
	}
	if month < 1 || month > 12 {
		return nil, errors.New("month must be 1-12")
	}
	if day < 1 || day > 31 {
		return nil, errors.New("day must be 1-31")
	}

	members, err := s.repo.ListByBirthdayMonthDay(month, day, string(models.LeadershipStatusApproved))
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

	result := &models.BirthdaySendResult{Targeted: len(members)}

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

func (s *leadershipService) AnniversaryStats() (*models.BirthdayStatsResponse, error) {
	counts, total, err := s.repo.AnniversaryCountsByMonth(string(models.LeadershipStatusApproved))
	if err != nil {
		return nil, err
	}
	months := make([]models.BirthdayMonthCount, 0, 12)
	for m := 1; m <= 12; m++ {
		months = append(months, models.BirthdayMonthCount{Month: m, Count: counts[m]})
	}
	return &models.BirthdayStatsResponse{
		Total:   total,
		ByMonth: months,
	}, nil
}

func (s *leadershipService) AnniversariesByMonth(month int) ([]models.LeadershipMember, error) {
	if month < 1 || month > 12 {
		return nil, errors.New("month must be 1-12")
	}
	return s.repo.ListByAnniversaryMonth(month, string(models.LeadershipStatusApproved))
}

func (s *leadershipService) AnniversariesToday(now time.Time) ([]models.LeadershipMember, error) {
	if now.IsZero() {
		now = time.Now()
	}
	return s.repo.ListByAnniversaryMonthDay(int(now.Month()), now.Day(), string(models.LeadershipStatusApproved))
}

func (s *leadershipService) SendAnniversaryGreetings(month, day int) (*models.BirthdaySendResult, error) {
	if s.sender == nil {
		return nil, errors.New("email sender is not configured")
	}
	if month < 1 || month > 12 {
		return nil, errors.New("month must be 1-12")
	}
	if day < 1 || day > 31 {
		return nil, errors.New("day must be 1-31")
	}

	members, err := s.repo.ListByAnniversaryMonthDay(month, day, string(models.LeadershipStatusApproved))
	if err != nil {
		return nil, err
	}

	appName := strings.TrimSpace(s.branding.AppName)
	if appName == "" {
		appName = "Wisdom House"
	}

	dateLabel := fmt.Sprintf("%02d/%02d", day, month)
	subject := fmt.Sprintf("Happy Wedding Anniversary from %s", appName)
	heroURL := email.TemplateAssetURL(s.branding, "anniversary", "hero.png")

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
		data := email.AnniversaryTemplateData{
			Branding:        s.branding,
			RecipientName:   fullName,
			AnniversaryDate: dateLabel,
			HeroImageURL:    heroURL,
		}

		body := ""
		if tplStore != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			_, htmlOut, _, err := tplStore.RenderWithData(ctx, "anniversary", data)
			cancel()
			if err == nil && strings.TrimSpace(htmlOut) != "" {
				body = htmlOut
			}
		}
		if strings.TrimSpace(body) == "" {
			body = email.RenderAnniversaryEmail(data)
		}

		if err := s.sender.SendHTML(addr, subject, body); err != nil {
			result.Skipped++
			continue
		}
		result.Sent++
	}

	return result, nil
}

func (s *leadershipService) createWithStatus(
	req *models.CreateLeadershipRequest,
	status models.LeadershipStatus,
) (*models.LeadershipMember, error) {
	firstName := strings.TrimSpace(req.FirstName)
	lastName := strings.TrimSpace(req.LastName)
	if firstName == "" || lastName == "" {
		return nil, errors.New("firstName and lastName are required")
	}
	if !isValidLeadershipRole(req.Role) {
		return nil, errors.New("invalid leadership role")
	}
	if !isValidLeadershipStatus(status) {
		return nil, errors.New("invalid leadership status")
	}

	bdayMonth, bdayDay, err := parseBirthday(req.BirthdayMonth, req.BirthdayDay, req.Birthday)
	if err != nil {
		return nil, err
	}
	annivMonth, annivDay, err := parseAnniversary(req.AnniversaryMonth, req.AnniversaryDay, req.Anniversary)
	if err != nil {
		return nil, err
	}

	member := &models.LeadershipMember{
		FirstName:        firstName,
		LastName:         lastName,
		Email:            optionalStringPtr(req.Email),
		Phone:            optionalStringPtr(req.Phone),
		Role:             req.Role,
		Status:           status,
		Bio:              req.Bio,
		ImageURL:         req.ImageURL,
		BirthdayMonth:    bdayMonth,
		BirthdayDay:      bdayDay,
		AnniversaryMonth: annivMonth,
		AnniversaryDay:   annivDay,
	}

	if err := s.repo.Create(member); err != nil {
		return nil, err
	}
	return member, nil
}

func (s *leadershipService) syncLeadershipIntakeSubmissions() {
	if s.formRepo == nil {
		return
	}

	submissions, err := s.formRepo.ListLeadershipIntakeSubmissions(500)
	if err != nil {
		return
	}

	for _, submission := range submissions {
		values, err := datatypesJSONToMap(submission.Values)
		if err != nil {
			continue
		}
		req, err := buildLeadershipRequest(values)
		if err != nil {
			continue
		}
		if strings.TrimSpace(req.Phone) == "" && submission.ContactNumber != nil {
			req.Phone = strings.TrimSpace(*submission.ContactNumber)
		}
		if strings.TrimSpace(req.Email) == "" && submission.Email != nil {
			req.Email = strings.TrimSpace(*submission.Email)
		}
		if strings.TrimSpace(req.Email) == "" {
			continue
		}
		if _, err := s.repo.GetByEmail(req.Email); err == nil {
			continue
		}
		_, _ = s.Apply(req)
	}
}

func datatypesJSONToMap(raw datatypes.JSON) (map[string]any, error) {
	out := map[string]any{}
	if len(raw) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *leadershipService) notifyNewApplication(member *models.LeadershipMember) {
	if s.notifySvc == nil || member == nil {
		return
	}
	fullName := strings.TrimSpace(strings.Join([]string{member.FirstName, member.LastName}, " "))
	title := "New leadership application"
	message := fmt.Sprintf("%s applied for leadership (%s).", fullName, humanLeadershipRole(member.Role))
	entityType := "leadership"
	entityID := member.ID
	_ = s.notifySvc.NotifyRoles(AdminNotificationInput{
		Type:       "leadership_application",
		Title:      title,
		Message:    message,
		EntityType: &entityType,
		EntityID:   &entityID,
		Roles:      []string{"super_admin"},
	})
}

func (s *leadershipService) notifyLeadershipDeleteRequest(member *models.LeadershipMember, req *models.ApprovalRequest) {
	if s.notifySvc == nil || member == nil || req == nil {
		return
	}
	fullName := strings.TrimSpace(strings.Join([]string{member.FirstName, member.LastName}, " "))
	if fullName == "" {
		fullName = "Leadership profile"
	}
	title := "Leadership delete approval required"
	message := fmt.Sprintf("%s was marked for deletion. Super admin approval is required before removal.", fullName)
	entityType := "leadership_delete"
	entityID := member.ID
	_ = s.notifySvc.NotifyRoles(AdminNotificationInput{
		Type:       "leadership_delete_request",
		Title:      title,
		Message:    message,
		EntityType: &entityType,
		EntityID:   &entityID,
		Roles:      []string{"super_admin"},
	})
}

func (s *leadershipService) sendLeadershipStatusEmail(member *models.LeadershipMember, status models.LeadershipStatus) {
	addr := ""
	if member != nil {
		addr = strings.TrimSpace(ptrString(member.Email))
	}
	if s.sender == nil || member == nil || addr == "" {
		return
	}

	templateKey := ""
	subject := ""
	appName := strings.TrimSpace(s.branding.AppName)
	if appName == "" {
		appName = "Wisdom House"
	}

	switch status {
	case models.LeadershipStatusApproved:
		templateKey = "leadership_approved"
		subject = fmt.Sprintf("%s leadership application approved", appName)
	case models.LeadershipStatusDeclined:
		templateKey = "leadership_declined"
		subject = fmt.Sprintf("%s leadership application update", appName)
	default:
		return
	}

	fullName := strings.TrimSpace(strings.Join([]string{member.FirstName, member.LastName}, " "))
	heroURL := email.TemplateAssetURL(s.branding, templateKey, "hero.png")
	data := email.LeadershipStatusTemplateData{
		Branding:      s.branding,
		RecipientName: fullName,
		Role:          humanLeadershipRole(member.Role),
		HeroImageURL:  heroURL,
	}

	body := ""
	if store, err := email.NewTemplateStoreFromEnv(); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		_, htmlOut, _, renderErr := store.RenderWithData(ctx, templateKey, data)
		cancel()
		if renderErr == nil && strings.TrimSpace(htmlOut) != "" {
			body = htmlOut
		}
	}

	if strings.TrimSpace(body) == "" {
		if status == models.LeadershipStatusApproved {
			body = email.RenderLeadershipApprovedEmail(data)
		} else {
			body = email.RenderLeadershipDeclinedEmail(data)
		}
	}

	_ = s.sender.SendHTML(addr, subject, body)
}

func isValidLeadershipRole(role models.LeadershipRole) bool {
	switch role {
	case models.LeadershipRoleSeniorPastor,
		models.LeadershipRoleAssociatePastor,
		models.LeadershipRoleDeacon,
		models.LeadershipRoleDeaconess,
		models.LeadershipRoleReverend:
		return true
	default:
		return false
	}
}

func isValidLeadershipStatus(status models.LeadershipStatus) bool {
	switch status {
	case models.LeadershipStatusPending,
		models.LeadershipStatusAwaitingSuperAdminApproval,
		models.LeadershipStatusApproved,
		models.LeadershipStatusDeclined:
		return true
	default:
		return false
	}
}

func humanLeadershipRole(role models.LeadershipRole) string {
	switch role {
	case models.LeadershipRoleSeniorPastor:
		return "Senior Pastor"
	case models.LeadershipRoleAssociatePastor:
		return "Associate Pastor"
	case models.LeadershipRoleDeacon:
		return "Deacon"
	case models.LeadershipRoleDeaconess:
		return "Deaconess"
	case models.LeadershipRoleReverend:
		return "Reverend"
	default:
		raw := strings.TrimSpace(string(role))
		if raw == "" {
			return "Leadership"
		}
		return strings.ReplaceAll(raw, "_", " ")
	}
}
