package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"wisdomHouse-backend/internal/authutil"
	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
	"wisdomHouse-backend/internal/sanitize"
)

type MemberService interface {
	List(page, limit int, active *bool) ([]models.Member, int64, error)
	Stats() (*models.MemberStatsResponse, error)
	NewMemberDashboard(now time.Time) (*models.NewMemberDashboardResponse, error)
	ListNewMemberSubmissions(page, limit int, start, end *time.Time) ([]models.NewMemberSubmission, int64, error)
	Create(req *models.CreateMemberRequest) (*models.Member, error)
	Update(id string, req *models.UpdateMemberRequest) (*models.Member, error)
	Delete(id string) error
	SendAnnouncement(req *models.SendMemberEmailRequest) (*models.BulkEmailResult, error)

	BirthdayStats() (*models.BirthdayStatsResponse, error)
	BirthdaysByMonth(month int) ([]models.Member, error)
	BirthdaysToday(now time.Time) ([]models.Member, error)
	SendBirthdayGreetings(month, day int) (*models.BirthdaySendResult, error)

	BulkImport(rows []CSVMemberRow) (*BulkImportResult, error)
}

type memberService struct {
	repo      repository.MemberRepository
	formRepo  repository.FormRepository
	eventRepo *repository.EventRepository
	sender    EmailSender
	branding  email.Branding
	protector *authutil.Protector
}

func NewMemberService(repo repository.MemberRepository, formRepo repository.FormRepository, eventRepo *repository.EventRepository, sender EmailSender, branding email.Branding, authSecret string) MemberService {
	var protector *authutil.Protector
	if p, err := authutil.NewProtector(authSecret); err == nil {
		protector = p
	}
	return &memberService{repo: repo, formRepo: formRepo, eventRepo: eventRepo, sender: sender, branding: branding, protector: protector}
}

// decryptMember decrypts PhoneEnc into Phone and clears the ciphertext field so
// it is never included in API responses. Falls back to the legacy plaintext phone
// column when no encrypted value is present (covers pre-migration records).
func (s *memberService) decryptMember(m *models.Member) {
	if m == nil || s.protector == nil {
		return
	}
	if m.PhoneEnc != nil {
		if dec, err := s.protector.DecryptString(*m.PhoneEnc); err == nil {
			m.Phone = &dec
		}
		m.PhoneEnc = nil
	}
}

func (s *memberService) decryptMembers(members []models.Member) {
	for i := range members {
		s.decryptMember(&members[i])
	}
}

func startOfWeek(t time.Time) time.Time {
	t = t.UTC()
	start := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	offset := (int(start.Weekday()) + 6) % 7
	return start.AddDate(0, 0, -offset)
}

func startOfQuarter(t time.Time) time.Time {
	t = t.UTC()
	quarterMonth := time.Month(((int(t.Month())-1)/3)*3 + 1)
	return time.Date(t.Year(), quarterMonth, 1, 0, 0, 0, 0, time.UTC)
}

func (s *memberService) List(page, limit int, active *bool) ([]models.Member, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 300 {
		limit = 10
	}
	offset := (page - 1) * limit
	members, total, err := s.repo.List(offset, limit, active)
	if err != nil {
		return nil, 0, err
	}
	s.decryptMembers(members)
	return members, total, nil
}

func (s *memberService) Stats() (*models.MemberStatsResponse, error) {
	return s.repo.Stats()
}

func (s *memberService) NewMemberDashboard(now time.Time) (*models.NewMemberDashboardResponse, error) {
	if s.formRepo == nil {
		return nil, errors.New("form repository not configured")
	}
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	weekStart := startOfWeek(now)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	quarterStart := startOfQuarter(now)
	yearStart := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	weekHistoryStart := now.AddDate(0, 0, -7*11)
	monthHistoryStart := now.AddDate(0, -11, 0)
	quarterHistoryStart := now.AddDate(0, -9, 0)
	yearHistoryStart := now.AddDate(-4, 0, 0)

	forms, err := s.formRepo.ListNewMemberForms()
	if err != nil {
		return nil, err
	}
	recent, _, err := s.formRepo.ListNewMemberSubmissions(0, 12, nil, nil)
	if err != nil {
		return nil, err
	}
	total, err := s.formRepo.CountNewMemberSubmissions(nil, nil)
	if err != nil {
		return nil, err
	}
	thisWeek, err := s.formRepo.CountNewMemberSubmissions(&weekStart, nil)
	if err != nil {
		return nil, err
	}
	thisMonth, err := s.formRepo.CountNewMemberSubmissions(&monthStart, nil)
	if err != nil {
		return nil, err
	}
	thisQuarter, err := s.formRepo.CountNewMemberSubmissions(&quarterStart, nil)
	if err != nil {
		return nil, err
	}
	thisYear, err := s.formRepo.CountNewMemberSubmissions(&yearStart, nil)
	if err != nil {
		return nil, err
	}
	weekly, err := s.formRepo.CountNewMemberSubmissionsByPeriod("week", &weekHistoryStart, nil)
	if err != nil {
		return nil, err
	}
	monthly, err := s.formRepo.CountNewMemberSubmissionsByPeriod("month", &monthHistoryStart, nil)
	if err != nil {
		return nil, err
	}
	quarterly, err := s.formRepo.CountNewMemberSubmissionsByPeriod("quarter", &quarterHistoryStart, nil)
	if err != nil {
		return nil, err
	}
	yearly, err := s.formRepo.CountNewMemberSubmissionsByPeriod("year", &yearHistoryStart, nil)
	if err != nil {
		return nil, err
	}

	return &models.NewMemberDashboardResponse{
		TotalSubmissions: total,
		ThisWeek:         thisWeek,
		ThisMonth:        thisMonth,
		ThisQuarter:      thisQuarter,
		ThisYear:         thisYear,
		Forms:            forms,
		Recent:           recent,
		WeeklyGrowth:     weekly,
		MonthlyGrowth:    monthly,
		QuarterlyGrowth:  quarterly,
		YearlyGrowth:     yearly,
	}, nil
}

func (s *memberService) ListNewMemberSubmissions(page, limit int, start, end *time.Time) ([]models.NewMemberSubmission, int64, error) {
	if s.formRepo == nil {
		return nil, 0, errors.New("form repository not configured")
	}
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 500 {
		limit = 25
	}
	offset := (page - 1) * limit
	return s.formRepo.ListNewMemberSubmissions(offset, limit, start, end)
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
		FirstName:     sanitize.Text(strings.TrimSpace(req.FirstName)),
		LastName:      sanitize.Text(strings.TrimSpace(req.LastName)),
		Email:         strings.ToLower(strings.TrimSpace(req.Email)),
		IsActive:      isActive,
		BirthdayMonth: month,
		BirthdayDay:   day,
	}

	if req.Phone != nil {
		phone := strings.TrimSpace(*req.Phone)
		if phone != "" && s.protector != nil {
			enc, err := s.protector.EncryptString(phone)
			if err != nil {
				return nil, errors.New("failed to encrypt phone number")
			}
			member.PhoneEnc = &enc
		}
	}

	if err := s.repo.Create(member); err != nil {
		return nil, err
	}
	s.decryptMember(member)
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
		phone := strings.TrimSpace(*req.Phone)
		if s.protector != nil {
			if phone != "" {
				enc, err := s.protector.EncryptString(phone)
				if err != nil {
					return nil, errors.New("failed to encrypt phone number")
				}
				updates["phone_enc"] = enc
			} else {
				updates["phone_enc"] = nil
			}
			updates["phone"] = nil // clear legacy plaintext column
		}
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

	member, err := s.repo.Update(id, updates)
	if err != nil {
		return nil, err
	}
	s.decryptMember(member)
	return member, nil
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
			Branding:      s.branding,
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
	members, err := s.repo.ListByMonth(month, true)
	if err != nil {
		return nil, err
	}
	s.decryptMembers(members)
	return members, nil
}

func (s *memberService) BirthdaysToday(now time.Time) ([]models.Member, error) {
	if now.IsZero() {
		now = time.Now()
	}
	month := int(now.Month())
	day := now.Day()
	members, err := s.repo.ListByMonthDay(month, day, true)
	if err != nil {
		return nil, err
	}
	s.decryptMembers(members)
	return members, nil
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
		appName = "The Wisdom Church"
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

// CSVMemberRow represents a single row parsed from a CSV import file.
// Column names are normalised to snake_case before mapping.
type CSVMemberRow struct {
	FirstName     string
	LastName      string
	Email         string
	Phone         string
	Birthday      string // DD/MM or YYYY-MM-DD
	IsActive      string // "true"/"false"/"1"/"0" — defaults to true
}

// BulkImportResult is the summary returned to the caller after a CSV import.
type BulkImportResult struct {
	Total    int            `json:"total"`
	Imported int            `json:"imported"`
	Skipped  int            `json:"skipped"`
	Errors   []string       `json:"errors,omitempty"`
}

func (s *memberService) BulkImport(rows []CSVMemberRow) (*BulkImportResult, error) {
	result := &BulkImportResult{Total: len(rows)}
	members := make([]models.Member, 0, len(rows))

	for i, row := range rows {
		first := strings.TrimSpace(row.FirstName)
		last := strings.TrimSpace(row.LastName)
		emailAddr := strings.TrimSpace(strings.ToLower(row.Email))

		if first == "" || last == "" || emailAddr == "" {
			result.Skipped++
			result.Errors = append(result.Errors, fmt.Sprintf("row %d: first_name, last_name and email are required", i+1))
			continue
		}

		m := models.Member{
			FirstName: first,
			LastName:  last,
			Email:     emailAddr,
			IsActive:  true,
		}

		// Phone encryption
		if phone := strings.TrimSpace(row.Phone); phone != "" && s.protector != nil {
			if enc, err := s.protector.EncryptString(phone); err == nil {
				m.PhoneEnc = &enc
			}
		}

		// Birthday parsing: DD/MM or YYYY-MM-DD
		if bday := strings.TrimSpace(row.Birthday); bday != "" {
			if month, day, err := parseBirthdayRow(bday); err == nil {
				m.BirthdayMonth = &month
				m.BirthdayDay = &day
			}
		}

		// Active flag
		switch strings.ToLower(strings.TrimSpace(row.IsActive)) {
		case "false", "0", "no", "inactive":
			m.IsActive = false
		}

		members = append(members, m)
	}

	if len(members) == 0 {
		return result, nil
	}

	imported, err := s.repo.BulkCreate(members)
	if err != nil {
		return result, fmt.Errorf("bulk import: %w", err)
	}
	result.Imported = imported
	return result, nil
}

func parseBirthdayRow(raw string) (month, day int, err error) {
	// Try DD/MM
	if _, e := fmt.Sscanf(raw, "%d/%d", &day, &month); e == nil && month >= 1 && month <= 12 && day >= 1 && day <= 31 {
		return month, day, nil
	}
	// Try YYYY-MM-DD
	var year int
	if _, e := fmt.Sscanf(raw, "%d-%d-%d", &year, &month, &day); e == nil && month >= 1 && month <= 12 && day >= 1 && day <= 31 {
		return month, day, nil
	}
	return 0, 0, fmt.Errorf("unrecognised birthday format %q", raw)
}

func ptrString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func optionalStringPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
