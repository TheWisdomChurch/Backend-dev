package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"gorm.io/gorm"

	"wisdomHouse-backend/internal/authutil"
	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

// WeddingAnniversaryService owns everything about "who is married to whom, when
// is their anniversary, may we email them, and are we still allowed to". It is
// the single source for the celebration automation's "anniversary" kind and for
// the manual admin send.
type WeddingAnniversaryService interface {
	UpsertForSubject(ctx context.Context, subjectType, subjectID string, in models.WeddingAnniversaryInput, source models.WeddingAnniversarySource, submissionID *string) (*models.WeddingAnniversary, error)
	Get(ctx context.Context, id string) (*models.WeddingAnniversary, error)
	List(ctx context.Context, f repository.WeddingAnniversaryFilter) ([]models.WeddingAnniversaryView, int64, error)
	Delete(ctx context.Context, id string) error
	Archive(ctx context.Context, id, reason string) error
	Unarchive(ctx context.Context, id string) error
	Stats(ctx context.Context) (*models.WeddingAnniversaryStats, error)
	DueOn(ctx context.Context, month, day int) ([]models.WeddingAnniversaryView, error)
	SendGreetingsForDay(ctx context.Context, month, day int) (*models.BirthdaySendResult, error)
}

type weddingAnniversaryService struct {
	repo      repository.WeddingAnniversaryRepository
	suppress  *repository.SubscriberRepository
	sender    EmailSender
	notifySvc AdminNotificationService
	branding  email.Branding
	protector *authutil.Protector
}

func NewWeddingAnniversaryService(
	repo repository.WeddingAnniversaryRepository,
	suppress *repository.SubscriberRepository,
	sender EmailSender,
	notifySvc AdminNotificationService,
	branding email.Branding,
	authSecret string,
) WeddingAnniversaryService {
	protector, _ := authutil.NewProtector(authSecret)
	return &weddingAnniversaryService{repo: repo, suppress: suppress, sender: sender, notifySvc: notifySvc, branding: branding, protector: protector}
}

// unsubscribeURL mirrors notificationService.unsubscribeURL / celebrationAutomationService.unsubscribeURL
// — same token scheme, same /notifications/unsubscribe endpoint, so any
// address (member or an external spouse who never subscribed to anything
// else) gets a working one-click unsubscribe.
func (s *weddingAnniversaryService) unsubscribeURL(address string) string {
	base := strings.TrimRight(strings.TrimSpace(s.branding.PublicURL), "/")
	if base == "" || s.protector == nil {
		return ""
	}
	token, err := s.protector.EncryptString("unsubscribe\n" + strings.ToLower(strings.TrimSpace(address)))
	if err != nil {
		return ""
	}
	return base + "/api/v1/notifications/unsubscribe?token=" + url.QueryEscape(token)
}

func (s *weddingAnniversaryService) UpsertForSubject(ctx context.Context, subjectType, subjectID string, in models.WeddingAnniversaryInput, source models.WeddingAnniversarySource, submissionID *string) (*models.WeddingAnniversary, error) {
	subjectType = strings.TrimSpace(subjectType)
	subjectID = strings.TrimSpace(subjectID)
	if !models.ValidWeddingAnniversarySubjectType(subjectType) || subjectID == "" {
		return nil, errors.New("valid subjectType and subjectId are required")
	}

	monthPtr, dayPtr, err := parseAnniversary(in.AnniversaryMonth, in.AnniversaryDay, in.Anniversary)
	if err != nil {
		return nil, err
	}
	if monthPtr == nil || dayPtr == nil {
		return nil, errors.New("a wedding anniversary date is required")
	}

	existing, getErr := s.repo.GetBySubject(ctx, subjectType, subjectID)
	if getErr != nil && !isRecordNotFound(getErr) {
		return nil, getErr
	}

	row := &models.WeddingAnniversary{
		SubjectType:      models.WeddingAnniversarySubjectType(subjectType),
		SubjectID:        subjectID,
		AnniversaryMonth: *monthPtr,
		AnniversaryDay:   *dayPtr,
		SpouseName:       titleCaseName(in.SpouseName),
		Status:           models.WeddingAnniversaryStatusActive,
		Source:           source,
	}
	if submissionID != nil && strings.TrimSpace(*submissionID) != "" {
		v := strings.TrimSpace(*submissionID)
		row.SourceSubmissionID = &v
	}
	if in.Notes != nil {
		row.Notes = cleanOptionalString(*in.Notes)
	}

	// Spouse resolution: an email that belongs to a person we already track
	// links them as an internal subject; otherwise they're an external contact.
	spouseEmail := strings.ToLower(strings.TrimSpace(valueOrEmpty(in.SpouseEmail)))
	if spouseEmail != "" {
		row.SpouseEmail = &spouseEmail
		row.SpouseEmailConsent = in.SpouseEmailConsent
		if st, sid, found, resolveErr := s.repo.ResolveSubjectByEmail(ctx, spouseEmail); resolveErr == nil && found {
			row.SpouseSubjectType = &st
			row.SpouseSubjectID = &sid
			row.SpouseIsExternal = false
		} else {
			row.SpouseIsExternal = true
		}
	} else if in.SpouseIsExternal != nil {
		row.SpouseIsExternal = *in.SpouseIsExternal
	}

	// Conflict policy: a human-entered date is never silently overwritten by a
	// later form/import write. Keep the stored date, still refresh spouse
	// details, and tell the admins so they can reconcile.
	if existing != nil {
		row.ID = existing.ID
		row.CreatedAt = existing.CreatedAt
		if existing.Status == models.WeddingAnniversaryStatusArchived {
			row.Status = existing.Status // don't silently un-archive
		}
		dateChanged := existing.AnniversaryMonth != row.AnniversaryMonth || existing.AnniversaryDay != row.AnniversaryDay
		nonAdminOverAdmin := source != models.WeddingAnniversarySourceAdmin && existing.Source == models.WeddingAnniversarySourceAdmin
		if dateChanged && nonAdminOverAdmin {
			row.AnniversaryMonth = existing.AnniversaryMonth
			row.AnniversaryDay = existing.AnniversaryDay
			s.notifyDateConflict(existing, *monthPtr, *dayPtr, source)
		}
		if strings.TrimSpace(row.SpouseName) == "" {
			row.SpouseName = existing.SpouseName
		}
	}

	return s.repo.Upsert(ctx, row)
}

func (s *weddingAnniversaryService) notifyDateConflict(existing *models.WeddingAnniversary, newMonth, newDay int, source models.WeddingAnniversarySource) {
	if s.notifySvc == nil {
		return
	}
	entityType := "wedding_anniversary"
	entityID := existing.ID
	_ = s.notifySvc.NotifyRoles(AdminNotificationInput{
		Type:  "wedding_anniversary_conflict",
		Title: "Wedding anniversary date conflict",
		Message: fmt.Sprintf(
			"A %s submission gave a different wedding anniversary (%02d/%02d) for a record already set to %02d/%02d. The stored date was kept — review and update if needed.",
			source, newDay, newMonth, existing.AnniversaryDay, existing.AnniversaryMonth,
		),
		EntityType: &entityType,
		EntityID:   &entityID,
		Roles:      []string{"admin", "super_admin"},
	})
}

func (s *weddingAnniversaryService) Get(ctx context.Context, id string) (*models.WeddingAnniversary, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *weddingAnniversaryService) List(ctx context.Context, f repository.WeddingAnniversaryFilter) ([]models.WeddingAnniversaryView, int64, error) {
	return s.repo.List(ctx, f)
}

func (s *weddingAnniversaryService) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func (s *weddingAnniversaryService) Archive(ctx context.Context, id, reason string) error {
	if err := s.repo.SetStatus(ctx, id, string(models.WeddingAnniversaryStatusArchived)); err != nil {
		return err
	}
	if strings.TrimSpace(reason) != "" {
		row, err := s.repo.GetByID(ctx, id)
		if err == nil {
			note := strings.TrimSpace(reason)
			if row.Notes != nil && strings.TrimSpace(*row.Notes) != "" {
				note = strings.TrimSpace(*row.Notes) + "\n" + note
			}
			row.Notes = &note
			_, _ = s.repo.Upsert(ctx, row)
		}
	}
	return nil
}

func (s *weddingAnniversaryService) Unarchive(ctx context.Context, id string) error {
	return s.repo.SetStatus(ctx, id, string(models.WeddingAnniversaryStatusActive))
}

func (s *weddingAnniversaryService) Stats(ctx context.Context) (*models.WeddingAnniversaryStats, error) {
	return s.repo.Stats(ctx)
}

func (s *weddingAnniversaryService) DueOn(ctx context.Context, month, day int) ([]models.WeddingAnniversaryView, error) {
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return nil, errors.New("invalid month or day")
	}
	return s.repo.ListDueByMonthDay(ctx, month, day)
}

func (s *weddingAnniversaryService) SendGreetingsForDay(ctx context.Context, month, day int) (*models.BirthdaySendResult, error) {
	if s.sender == nil {
		return nil, errors.New("email sender is not configured")
	}
	views, err := s.DueOn(ctx, month, day)
	if err != nil {
		return nil, err
	}

	suppressed := map[string]bool{}
	if s.suppress != nil {
		if emails, listErr := s.suppress.ListUnsubscribedEmails(); listErr == nil {
			for _, e := range emails {
				suppressed[strings.ToLower(strings.TrimSpace(e))] = true
			}
		}
	}

	appName := strings.TrimSpace(s.branding.AppName)
	if appName == "" {
		appName = "The Wisdom Church"
	}
	subject := fmt.Sprintf("Happy Wedding Anniversary from %s", appName)
	heroURL := email.TemplateAssetURL(s.branding, "anniversary", "hero.png")
	dateLabel := fmt.Sprintf("%02d/%02d", day, month)

	result := &models.BirthdaySendResult{Targeted: len(views)}
	sentTo := map[string]bool{}

	for i := range views {
		greeting := coupleGreetingName(views[i])
		body := email.RenderAnniversaryEmail(email.AnniversaryTemplateData{
			Branding:        s.branding,
			RecipientName:   greeting,
			SpouseName:      titleCaseName(views[i].SpouseName),
			AnniversaryDate: dateLabel,
			HeroImageURL:    heroURL,
		})

		anySent := false
		for _, addr := range coupleAddresses(views[i]) {
			addr = strings.ToLower(strings.TrimSpace(addr))
			if addr == "" || suppressed[addr] || sentTo[addr] {
				continue
			}
			sentTo[addr] = true
			if sendErr := sendCelebrationEmail(s.sender, addr, subject, body, s.unsubscribeURL(addr)); sendErr != nil {
				continue
			}
			anySent = true
		}
		if anySent {
			result.Sent++
		} else {
			result.Skipped++
		}
	}
	return result, nil
}

// coupleGreetingName renders "David & Sarah" when a spouse name is known, else
// just the greeted person's name.
func coupleGreetingName(v models.WeddingAnniversaryView) string {
	primary := personDisplayName(v.FirstName, v.LastName)
	spouse := titleCaseName(v.SpouseName)
	if primary != "" && spouse != "" {
		return primary + " & " + spouse
	}
	if primary != "" {
		return primary
	}
	return spouse
}

// coupleAddresses is the set of addresses that should receive the greeting: the
// greeted person, plus the spouse only when their email is on file AND they
// consented.
func coupleAddresses(v models.WeddingAnniversaryView) []string {
	addrs := []string{}
	if e := strings.TrimSpace(v.Email); e != "" {
		addrs = append(addrs, e)
	}
	if v.SpouseEmailConsent && v.SpouseEmail != nil {
		if e := strings.TrimSpace(*v.SpouseEmail); e != "" {
			addrs = append(addrs, e)
		}
	}
	return addrs
}

func isRecordNotFound(err error) bool {
	return err != nil && errors.Is(err, gorm.ErrRecordNotFound)
}
