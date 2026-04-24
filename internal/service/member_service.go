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

type MemberService interface {
	List(page, limit int, active *bool) ([]models.Member, int64, error)
	Create(req *models.CreateMemberRequest) (*models.Member, error)
	Update(id string, req *models.UpdateMemberRequest) (*models.Member, error)
	Delete(id string) error
	SendAnnouncement(req *models.SendMemberEmailRequest) (*models.BulkEmailResult, error)

	BirthdayStats() (*models.BirthdayStatsResponse, error)
	BirthdaysByMonth(month int) ([]models.Member, error)
	BirthdaysToday(now time.Time) ([]models.Member, error)
	SendBirthdayGreetings(month, day int) (*models.BirthdaySendResult, error)
}

type memberService struct {
	repo      repository.MemberRepository
	eventRepo *repository.EventRepository
	sender    EmailSender
	branding  email.Branding
}

func NewMemberService(repo repository.MemberRepository, eventRepo *repository.EventRepository, sender EmailSender, branding email.Branding) MemberService {
	return &memberService{repo: repo, eventRepo: eventRepo, sender: sender, branding: branding}
}

func (s *memberService) List(page, limit int, active *bool) ([]models.Member, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 300 {
		limit = 10
	}
	offset := (page - 1) * limit
	return s.repo.List(offset, limit, active)
}

func (s *memberService) Create(req *models.CreateMemberRequest) (*models.Member, error) {
	if strings.TrimSpace(req.FirstName) == "" || strings.TrimSpace(req.LastName) == "" {
		return nil, errors.New("firstName and lastName are required")
	}
	if strings.TrimSpace(req.Email) == "" {
		return nil, errors.New("email is required")
	}

	month, day, err := parseBirthday(req.BirthdayMonth, req.BirthdayDay, req.Birthday)
	if err != nil {
		return nil, err
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	member := &models.Member{
		FirstName:     strings.TrimSpace(req.FirstName),
		LastName:      strings.TrimSpace(req.LastName),
		Email:         strings.TrimSpace(req.Email),
		Phone:         strings.TrimSpace(ptrString(req.Phone)),
		IsActive:      isActive,
		BirthdayMonth: month,
		BirthdayDay:   day,
	}

	if err := s.repo.Create(member); err != nil {
		return nil, err
	}
	return member, nil
}

func (s *memberService) Update(id string, req *models.UpdateMemberRequest) (*models.Member, error) {
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
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
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

func (s *memberService) Delete(id string) error {
	return s.repo.Delete(id)
}

func (s *memberService) SendAnnouncement(req *models.SendMemberEmailRequest) (*models.BulkEmailResult, error) {
	if s.sender == nil {
		return nil, errors.New("email sender is not configured")
	}
	if req == nil {
		return nil, errors.New("request is required")
	}
	subject := strings.TrimSpace(req.Subject)
	message := strings.TrimSpace(req.Message)
	if subject == "" || message == "" {
		return nil, errors.New("subject and message are required")
	}

	activeOnly := true
	if req.OnlyActive != nil {
		activeOnly = *req.OnlyActive
	}

	members, err := s.repo.ListAll(&activeOnly)
	if err != nil {
		return nil, err
	}

	var event *models.Event
	if req.EventID != nil && s.eventRepo != nil {
		if ev, err := s.eventRepo.GetByID(strings.TrimSpace(*req.EventID)); err == nil {
			event = ev
		}
	}

	result := &models.BulkEmailResult{Targeted: len(members)}
	for i := range members {
		addr := strings.TrimSpace(members[i].Email)
		if addr == "" {
			result.Skipped++
			continue
		}
		fullName := strings.TrimSpace(strings.Join([]string{members[i].FirstName, members[i].LastName}, " "))
		var namePtr *string
		if fullName != "" {
			namePtr = &fullName
		}

		body := email.RenderNotificationEmail(email.NotificationTemplateData{
			Title:         subject,
			Message:       message,
			Event:         event,
			RecipientName: namePtr,
		})

		if err := s.sender.SendHTML(addr, subject, body); err != nil {
			result.Skipped++
			continue
		}
		result.Sent++
	}

	return result, nil
}

func (s *memberService) BirthdayStats() (*models.BirthdayStatsResponse, error) {
	counts, total, err := s.repo.BirthdayCountsByMonth(true)
	if err != nil {
		return nil, err
	}

	months := make([]models.BirthdayMonthCount, 0, 12)
	for m := 1; m <= 12; m++ {
		months = append(months, models.BirthdayMonthCount{Month: m, Count: counts[m]})
	}

	return &models.BirthdayStatsResponse{Total: total, ByMonth: months}, nil
}

func (s *memberService) BirthdaysByMonth(month int) ([]models.Member, error) {
	if month < 1 || month > 12 {
		return nil, errors.New("month must be 1-12")
	}
	return s.repo.ListByMonth(month, true)
}

func (s *memberService) BirthdaysToday(now time.Time) ([]models.Member, error) {
	if now.IsZero() {
		now = time.Now()
	}
	month := int(now.Month())
	day := now.Day()
	return s.repo.ListByMonthDay(month, day, true)
}

func (s *memberService) SendBirthdayGreetings(month, day int) (*models.BirthdaySendResult, error) {
	if s.sender == nil {
		return nil, errors.New("email sender is not configured")
	}
	if month < 1 || month > 12 {
		return nil, errors.New("month must be 1-12")
	}
	if day < 1 || day > 31 {
		return nil, errors.New("day must be 1-31")
	}

	members, err := s.repo.ListByMonthDay(month, day, true)
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
		addr := strings.TrimSpace(members[i].Email)
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

func ptrString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
