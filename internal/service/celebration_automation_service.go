package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"wisdomHouse-backend/internal/authutil"
	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

type CelebrationAutomationService interface {
	GetStatus(time.Time) (*models.CelebrationAutomationStatus, error)
	UpdateConfig(*models.UpdateCelebrationAutomationConfigRequest, *models.AdminEmailSendActor) (*models.CelebrationAutomationConfig, error)
	ProcessDue(context.Context, time.Time, string, string) (*models.CelebrationAutomationRun, error)
	ListRuns(int, int) ([]models.CelebrationAutomationRun, int64, error)
	ListDeliveries(string, int, int) ([]models.CelebrationDelivery, int64, error)
}

type celebrationAutomationService struct {
	repo        repository.CelebrationAutomationRepository
	subscribers *repository.SubscriberRepository
	sender      EmailSender
	branding    email.Branding
	protector   *authutil.Protector
}

func NewCelebrationAutomationService(repo repository.CelebrationAutomationRepository, subscribers *repository.SubscriberRepository, sender EmailSender, branding email.Branding, authSecret string) CelebrationAutomationService {
	protector, _ := authutil.NewProtector(authSecret)
	return &celebrationAutomationService{repo: repo, subscribers: subscribers, sender: sender, branding: branding, protector: protector}
}

func (s *celebrationAutomationService) GetStatus(now time.Time) (*models.CelebrationAutomationStatus, error) {
	config, err := s.repo.GetConfig()
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(config.Timezone)
	if err != nil {
		return nil, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	local := now.In(loc)
	date := local.Format("2006-01-02")
	run, runErr := s.repo.GetRunByDate(date)
	if errors.Is(runErr, gorm.ErrRecordNotFound) {
		run = nil
	} else if runErr != nil {
		return nil, runErr
	}
	next := nextCelebrationRun(local, config.SendTime)
	healthy := config.LastWorkerHeartbeat != nil && now.UTC().Sub(config.LastWorkerHeartbeat.UTC()) <= 3*time.Minute
	return &models.CelebrationAutomationStatus{Config: *config, TodayRun: run, NextRunAt: &next, WorkerHealthy: healthy}, nil
}

func (s *celebrationAutomationService) UpdateConfig(req *models.UpdateCelebrationAutomationConfigRequest, actor *models.AdminEmailSendActor) (*models.CelebrationAutomationConfig, error) {
	if req == nil {
		return nil, errors.New("request is required")
	}
	config := &models.CelebrationAutomationConfig{ID: "default", Enabled: req.Enabled, BirthdayEnabled: req.BirthdayEnabled, AnniversaryEnabled: req.AnniversaryEnabled, Timezone: strings.TrimSpace(req.Timezone), SendTime: strings.TrimSpace(req.SendTime), Feb29Policy: strings.TrimSpace(req.Feb29Policy), MaxAttempts: req.MaxAttempts, RetryMinutes: req.RetryMinutes, BirthdaySubject: strings.TrimSpace(req.BirthdaySubject), AnniversarySubject: strings.TrimSpace(req.AnniversarySubject), BirthdayTemplateKey: strings.Trim(strings.TrimSpace(req.BirthdayTemplateKey), "/"), AnniversaryTemplateKey: strings.Trim(strings.TrimSpace(req.AnniversaryTemplateKey), "/")}
	if err := configValid(config); err != nil {
		return nil, err
	}
	if actor != nil {
		if actor.UserID != "" {
			config.UpdatedByUserID = &actor.UserID
		}
		if actor.Email != "" {
			config.UpdatedByEmail = &actor.Email
		}
	}
	if err := s.repo.UpdateConfig(config); err != nil {
		return nil, err
	}
	return s.repo.GetConfig()
}

func configValid(v *models.CelebrationAutomationConfig) error {
	if v == nil {
		return errors.New("configuration is required")
	}
	if _, err := time.LoadLocation(v.Timezone); err != nil {
		return errors.New("timezone must be a valid IANA timezone")
	}
	if _, err := time.Parse("15:04", v.SendTime); err != nil {
		return errors.New("sendTime must use HH:mm")
	}
	if v.Feb29Policy != "feb28" && v.Feb29Policy != "mar1" && v.Feb29Policy != "leap_only" {
		return errors.New("feb29Policy must be feb28, mar1, or leap_only")
	}
	if v.MaxAttempts < 1 || v.MaxAttempts > 10 {
		return errors.New("maxAttempts must be 1-10")
	}
	if v.RetryMinutes < 1 || v.RetryMinutes > 1440 {
		return errors.New("retryMinutes must be 1-1440")
	}
	for field, value := range map[string]string{"birthdaySubject": v.BirthdaySubject, "anniversarySubject": v.AnniversarySubject} {
		if value == "" || utf8.RuneCountInString(value) > 180 {
			return fmt.Errorf("%s is required and must be 180 characters or fewer", field)
		}
	}
	for field, value := range map[string]string{"birthdayTemplateKey": v.BirthdayTemplateKey, "anniversaryTemplateKey": v.AnniversaryTemplateKey} {
		if value == "" || len(value) > 120 || strings.Contains(value, "..") || !templateKeyRe.MatchString(value) {
			return fmt.Errorf("%s is invalid", field)
		}
	}
	return nil
}

func nextCelebrationRun(local time.Time, sendTime string) time.Time {
	clock, _ := time.Parse("15:04", sendTime)
	next := time.Date(local.Year(), local.Month(), local.Day(), clock.Hour(), clock.Minute(), 0, 0, local.Location())
	if !next.After(local) {
		next = time.Date(local.Year(), local.Month(), local.Day()+1, clock.Hour(), clock.Minute(), 0, 0, local.Location())
	}
	return next
}

func (s *celebrationAutomationService) ProcessDue(ctx context.Context, now time.Time, worker, trigger string) (*models.CelebrationAutomationRun, error) {
	if s == nil || s.repo == nil || s.sender == nil {
		return nil, errors.New("celebration automation is not configured")
	}
	worker = strings.TrimSpace(worker)
	if worker == "" {
		return nil, errors.New("worker id is required")
	}
	if now.IsZero() {
		now = time.Now()
	}
	config, err := s.repo.GetConfig()
	if err != nil {
		return nil, err
	}
	if err := configValid(config); err != nil {
		return nil, err
	}
	if touchErr := s.repo.TouchWorker(ctx, worker, now.UTC()); touchErr != nil {
		return nil, fmt.Errorf("record worker heartbeat: %w", touchErr)
	}
	loc, _ := time.LoadLocation(config.Timezone)
	local := now.In(loc)
	if trigger == "scheduler" {
		if !config.Enabled {
			return nil, nil
		}
		clock, _ := time.Parse("15:04", config.SendTime)
		scheduled := time.Date(local.Year(), local.Month(), local.Day(), clock.Hour(), clock.Minute(), 0, 0, loc)
		if local.Before(scheduled) {
			return nil, nil
		}
	}
	snapshot, _ := json.Marshal(config)
	run, err := s.repo.EnsureRun(ctx, local.Format("2006-01-02"), config.Timezone, trigger, snapshot)
	if err != nil {
		return nil, err
	}
	claimed, err := s.repo.ClaimRun(ctx, run.ID, worker, now.UTC())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if trigger == "manual" {
			return run, nil
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.executeRun(ctx, claimed, config, local, worker)
}

type celebrationRecipient struct {
	email, name, kind string
	sources           []map[string]string
}

func (s *celebrationAutomationService) executeRun(ctx context.Context, run *models.CelebrationAutomationRun, config *models.CelebrationAutomationConfig, local time.Time, worker string) (*models.CelebrationAutomationRun, error) {
	stopHeartbeat := make(chan struct{})
	var heartbeat sync.WaitGroup
	heartbeat.Add(1)
	go func() {
		defer heartbeat.Done()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-stopHeartbeat:
				return
			case <-ctx.Done():
				return
			case tick := <-ticker.C:
				_, _ = s.repo.RenewRunClaim(ctx, run.ID, worker, tick.UTC())
			}
		}
	}()
	defer func() { close(stopHeartbeat); heartbeat.Wait() }()
	candidates, err := s.repo.ListCandidates(ctx, int(local.Month()), local.Day(), config.BirthdayEnabled, config.AnniversaryEnabled)
	if err != nil {
		return nil, s.failRun(ctx, run, config, worker, err)
	}
	if config.BirthdayEnabled && shouldIncludeFeb29(local, config.Feb29Policy) {
		extra, extraErr := s.repo.ListCandidates(ctx, 2, 29, true, false)
		if extraErr != nil {
			return nil, s.failRun(ctx, run, config, worker, extraErr)
		}
		candidates = append(candidates, extra...)
	}
	aggregated := map[string]*celebrationRecipient{}
	invalid := 0
	for _, candidate := range candidates {
		address := normalizeEmail(candidate.Email)
		if address == "" || !emailRe.MatchString(address) {
			invalid++
			continue
		}
		key := candidate.Kind + "|" + address
		item := aggregated[key]
		name := strings.TrimSpace(strings.Join([]string{candidate.FirstName, candidate.LastName}, " "))
		if item == nil {
			item = &celebrationRecipient{email: address, name: name, kind: candidate.Kind}
			aggregated[key] = item
		} else if item.name == "" {
			item.name = name
		}
		item.sources = append(item.sources, map[string]string{"type": candidate.Source, "id": candidate.SourceID})
	}
	suppressed := map[string]bool{}
	if s.subscribers != nil {
		emails, suppressErr := s.subscribers.ListUnsubscribedEmails()
		if suppressErr != nil {
			return nil, s.failRun(ctx, run, config, worker, suppressErr)
		}
		for _, address := range emails {
			suppressed[normalizeEmail(address)] = true
		}
	}
	keys := make([]string, 0, len(aggregated))
	for key := range aggregated {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var tplStore *email.TemplateStore
	if store, storeErr := email.NewTemplateStoreFromEnv(); storeErr == nil {
		tplStore = store
	}
	run.Targeted = len(aggregated)
	run.Skipped = invalid
	run.Sent = 0
	run.Suppressed = 0
	run.Failed = 0
	var deliveryErrors []string
	for _, key := range keys {
		recipient := aggregated[key]
		sourceJSON, _ := json.Marshal(recipient.sources)
		hash := sha256.Sum256([]byte(recipient.email))
		delivery, deliveryErr := s.repo.UpsertDelivery(ctx, &models.CelebrationDelivery{RunID: run.ID, Kind: recipient.kind, EmailHash: hex.EncodeToString(hash[:]), RecipientEmail: recipient.email, RecipientName: recipient.name, Sources: datatypes.JSON(sourceJSON), Status: "pending"})
		if deliveryErr != nil {
			return nil, s.failRun(ctx, run, config, worker, deliveryErr)
		}
		if delivery.Status == "sent" {
			run.Sent++
			continue
		}
		if delivery.Status == "suppressed" {
			run.Suppressed++
			continue
		}
		if delivery.Attempt >= config.MaxAttempts {
			run.Failed++
			continue
		}
		delivery.Attempt++
		delivery.Sources = datatypes.JSON(sourceJSON)
		delivery.RecipientName = recipient.name
		if suppressed[recipient.email] {
			delivery.Status = "suppressed"
			delivery.Error = nil
			_ = s.repo.UpdateDelivery(ctx, delivery)
			run.Suppressed++
			continue
		}
		body, subject := s.renderCelebration(ctx, tplStore, config, recipient, local)
		unsubscribeURL := s.unsubscribeURL(recipient.email)
		sendErr := sendCelebrationEmail(s.sender, recipient.email, subject, body, unsubscribeURL)
		if sendErr != nil {
			message := sendErr.Error()
			delivery.Status = "failed"
			delivery.Error = &message
			run.Failed++
			deliveryErrors = append(deliveryErrors, maskAutomationEmail(recipient.email)+": "+message)
		} else {
			sentAt := time.Now().UTC()
			delivery.Status = "sent"
			delivery.Error = nil
			delivery.SentAt = &sentAt
			run.Sent++
		}
		if updateErr := s.repo.UpdateDelivery(ctx, delivery); updateErr != nil {
			return nil, s.failRun(ctx, run, config, worker, updateErr)
		}
	}
	completed := time.Now().UTC()
	run.CompletedAt = &completed
	run.NextAttemptAt = nil
	run.LastError = nil
	if run.Failed > 0 {
		message := strings.Join(deliveryErrors, "; ")
		if len(message) > 2000 {
			message = message[:2000]
		}
		run.LastError = &message
		if run.Attempt < config.MaxAttempts {
			run.Status = "partial"
			next := completed.Add(time.Duration(config.RetryMinutes) * time.Minute)
			run.NextAttemptAt = &next
		} else {
			run.Status = "failed"
		}
	} else {
		run.Status = "completed"
	}
	if err := s.repo.CompleteRun(ctx, run, worker); err != nil {
		return nil, err
	}
	return run, nil
}

func shouldIncludeFeb29(local time.Time, policy string) bool {
	if time.Date(local.Year(), 3, 0, 0, 0, 0, 0, local.Location()).Day() == 29 {
		return false
	}
	return (policy == "feb28" && local.Month() == 2 && local.Day() == 28) || (policy == "mar1" && local.Month() == 3 && local.Day() == 1)
}
func (s *celebrationAutomationService) failRun(ctx context.Context, run *models.CelebrationAutomationRun, config *models.CelebrationAutomationConfig, worker string, cause error) error {
	message := cause.Error()
	run.LastError = &message
	completed := time.Now().UTC()
	run.CompletedAt = &completed
	if run.Attempt < config.MaxAttempts {
		run.Status = "partial"
		next := completed.Add(time.Duration(config.RetryMinutes) * time.Minute)
		run.NextAttemptAt = &next
	} else {
		run.Status = "failed"
	}
	if err := s.repo.CompleteRun(ctx, run, worker); err != nil {
		return fmt.Errorf("%v; persist run failure: %w", cause, err)
	}
	return cause
}
func (s *celebrationAutomationService) renderCelebration(ctx context.Context, store *email.TemplateStore, config *models.CelebrationAutomationConfig, recipient *celebrationRecipient, local time.Time) (string, string) {
	dateLabel := local.Format("02/01")
	if recipient.kind == "anniversary" {
		data := email.AnniversaryTemplateData{Branding: s.branding, RecipientName: recipient.name, AnniversaryDate: dateLabel, HeroImageURL: email.TemplateAssetURL(s.branding, "anniversary", "hero.png")}
		if store != nil {
			renderCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			_, html, _, err := store.RenderWithData(renderCtx, config.AnniversaryTemplateKey, data)
			cancel()
			if err == nil && strings.TrimSpace(html) != "" {
				return html, config.AnniversarySubject
			}
		}
		return email.RenderAnniversaryEmail(data), config.AnniversarySubject
	}
	data := email.BirthdayTemplateData{Branding: s.branding, RecipientName: recipient.name, BirthdayDate: dateLabel, HeroImageURL: email.TemplateAssetURL(s.branding, "birthday", "hero.png")}
	if store != nil {
		renderCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		_, html, _, err := store.RenderWithData(renderCtx, config.BirthdayTemplateKey, data)
		cancel()
		if err == nil && strings.TrimSpace(html) != "" {
			return html, config.BirthdaySubject
		}
	}
	return email.RenderBirthdayEmail(data), config.BirthdaySubject
}
func (s *celebrationAutomationService) unsubscribeURL(address string) string {
	base := strings.TrimRight(strings.TrimSpace(s.branding.PublicURL), "/")
	if base == "" || s.protector == nil {
		return ""
	}
	token, err := s.protector.EncryptString("unsubscribe\n" + normalizeEmail(address))
	if err != nil {
		return ""
	}
	return base + "/api/v1/notifications/unsubscribe?token=" + url.QueryEscape(token)
}
func sendCelebrationEmail(sender EmailSender, to, subject, body, unsubscribeURL string) error {
	if capable, ok := sender.(interface {
		SendHTMLTextWithOptions(string, string, string, string, email.MessageOptions) error
	}); ok {
		return capable.SendHTMLTextWithOptions(to, subject, body, "", email.MessageOptions{UnsubscribeURL: unsubscribeURL})
	}
	return sender.SendHTML(to, subject, body)
}
func maskAutomationEmail(address string) string {
	parts := strings.Split(normalizeEmail(address), "@")
	if len(parts) != 2 {
		return "invalid"
	}
	return parts[0][:min(2, len(parts[0]))] + "***@" + parts[1]
}
func (s *celebrationAutomationService) ListRuns(page, limit int) ([]models.CelebrationAutomationRun, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return s.repo.ListRuns((page-1)*limit, limit)
}
func (s *celebrationAutomationService) ListDeliveries(runID string, page, limit int) ([]models.CelebrationDelivery, int64, error) {
	if strings.TrimSpace(runID) == "" {
		return nil, 0, errors.New("run id is required")
	}
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	return s.repo.ListDeliveries(runID, (page-1)*limit, limit)
}
