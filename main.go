// cmd/api/main.go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"wisdomHouse-backend/internal/cache"
	"wisdomHouse-backend/internal/config"
	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/handlers"
	"wisdomHouse-backend/internal/middleware"
	"wisdomHouse-backend/internal/repository"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/validation"
)

// @title Wisdom House Backend API
// @version 1.0.0
// @description Backend API for Wisdom House Church
// @host localhost:8080
// @BasePath /api/v1

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

func isTrueEnv(key string) bool {
	val := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch val {
	case "1", "true", "t", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func hasAnyEnv(keys ...string) bool {
	for _, key := range keys {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return true
		}
	}
	return false
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

func ensureCORSDefaults(cfg *config.Config) {
	if cfg == nil {
		return
	}

	// Otherwise, ensure app URLs are allowed (useful for production).
	existing := make(map[string]struct{}, len(cfg.CORS.AllowedOrigins))
	for _, o := range cfg.CORS.AllowedOrigins {
		o = normalizeCORSOrigin(o)
		if o == "" {
			continue
		}
		existing[o] = struct{}{}
	}

	candidates := []string{
		normalizeCORSOrigin(cfg.App.FrontendURL),
		normalizeCORSOrigin(cfg.App.AdminPortalURL),
		"https://wisdomchurchhq.org",
		"https://www.wisdomchurchhq.org",
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if _, ok := existing[c]; ok {
			continue
		}
		cfg.CORS.AllowedOrigins = append(cfg.CORS.AllowedOrigins, c)
		existing[c] = struct{}{}
	}

	if len(cfg.CORS.AllowedOrigins) == 0 {
		cfg.CORS.AllowedOrigins = []string{"http://localhost:3000", "http://localhost:3001"}
	}
}

func normalizeCORSOrigin(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	if strings.Contains(value, "://") {
		return ""
	}
	return "https://" + value
}

type chainedEmailSender struct {
	primary      service.EmailSender
	fallback     service.EmailSender
	primaryName  string
	fallbackName string
	logger       *log.Logger
}

type observedEmailSender struct {
	inner  service.EmailSender
	logger *log.Logger
}

func (s observedEmailSender) SendHTML(to, subject, body string) error {
	if s.inner == nil {
		return nil
	}
	err := s.inner.SendHTML(to, subject, body)
	if err != nil && s.logger != nil {
		s.logger.Printf("❌ Email delivery failed to=%s subject=%q error=%v", maskEmail(to), subject, err)
	}
	return err
}

func (s observedEmailSender) SendHTMLText(to, subject, htmlBody, textBody string) error {
	if s.inner == nil {
		return nil
	}
	if multipart, ok := s.inner.(interface {
		SendHTMLText(to, subject, htmlBody, textBody string) error
	}); ok {
		err := multipart.SendHTMLText(to, subject, htmlBody, textBody)
		if err != nil && s.logger != nil {
			s.logger.Printf("❌ Multipart email delivery failed to=%s subject=%q error=%v", maskEmail(to), subject, err)
		}
		return err
	}
	err := s.inner.SendHTML(to, subject, htmlBody)
	if err != nil && s.logger != nil {
		s.logger.Printf("❌ HTML email delivery failed to=%s subject=%q error=%v", maskEmail(to), subject, err)
	}
	return err
}

func maskEmail(raw string) string {
	email := strings.TrimSpace(strings.ToLower(raw))
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "invalid-email"
	}
	local := parts[0]
	domain := parts[1]
	if local == "" {
		return "***@" + domain
	}
	if len(local) <= 2 {
		return local[:1] + "***@" + domain
	}
	return local[:2] + "***@" + domain
}

func (s chainedEmailSender) SendHTML(to, subject, body string) error {
	if s.primary == nil {
		if s.fallback != nil {
			return s.fallback.SendHTML(to, subject, body)
		}
		return nil
	}
	if err := s.primary.SendHTML(to, subject, body); err != nil {
		if s.fallback != nil {
			if s.logger != nil {
				s.logger.Printf("⚠️ Email send via %s failed: %v. Falling back to %s", s.primaryName, err, s.fallbackName)
			}
			if err2 := s.fallback.SendHTML(to, subject, body); err2 == nil {
				return nil
			} else {
				return fmt.Errorf("%s failed: %w; %s failed: %v", s.primaryName, err, s.fallbackName, err2)
			}
		}
		return err
	}
	return nil
}

func (s chainedEmailSender) SendHTMLText(to, subject, htmlBody, textBody string) error {
	sendMultipart := func(sender service.EmailSender) error {
		if sender == nil {
			return nil
		}
		if multipart, ok := sender.(interface {
			SendHTMLText(to, subject, htmlBody, textBody string) error
		}); ok {
			return multipart.SendHTMLText(to, subject, htmlBody, textBody)
		}
		return sender.SendHTML(to, subject, htmlBody)
	}

	if s.primary == nil {
		if s.fallback != nil {
			return sendMultipart(s.fallback)
		}
		return nil
	}
	if err := sendMultipart(s.primary); err != nil {
		if s.fallback != nil {
			if s.logger != nil {
				s.logger.Printf("⚠️ Email send via %s failed: %v. Falling back to %s", s.primaryName, err, s.fallbackName)
			}
			if err2 := sendMultipart(s.fallback); err2 == nil {
				return nil
			} else {
				return fmt.Errorf("%s failed: %w; %s failed: %v", s.primaryName, err, s.fallbackName, err2)
			}
		}
		return err
	}
	return nil
}

func initEmailSender(cfg *config.Config, logger *log.Logger) service.EmailSender {
	if cfg == nil {
		return noopEmailSender{}
	}

	var primary service.EmailSender
	var fallback service.EmailSender
	var primaryName string
	var fallbackName string

	// Prefer SMTP (Postfix/local relay) if configured.
	if strings.TrimSpace(cfg.SMTP.Host) != "" {
		s, err := email.NewSender(
			cfg.Redis.URL,
			cfg.SMTP.Host,
			cfg.SMTP.Port,
			cfg.SMTP.User,
			cfg.SMTP.Password,
			cfg.SMTP.From,
			cfg.SMTP.TLS,
		)
		if err != nil {
			logger.Printf("⚠️ SMTP sender not initialized: %v", err)
		} else {
			primary = s
			primaryName = "SMTP"
			logger.Println("✅ Email sender initialized (SMTP relay)")
		}
	}

	// Next: Brevo
	if hasAnyEnv("BREVO_API_KEY", "BREVO_FROM_EMAIL", "BREVO_FROM_NAME", "BREVO_BASE_URL") {
		s, err := email.NewBrevoSender(cfg.Redis.URL, "", "", "", "")
		if err != nil {
			logger.Printf("⚠️ Brevo email sender not initialized: %v", err)
		} else if primary == nil {
			primary = s
			primaryName = "Brevo"
			logger.Println("✅ Email sender initialized (Brevo API)")
		} else if fallback == nil {
			fallback = s
			fallbackName = "Brevo"
			logger.Println("✅ Email fallback configured (Brevo API)")
		}
	}

	if primary == nil {
		logger.Println("⚠️ Email sender not configured (no SMTP/Brevo/SES). Outbound email disabled.")
		return noopEmailSender{}
	}
	if fallback == nil {
		return primary
	}
	return chainedEmailSender{
		primary:      primary,
		fallback:     fallback,
		primaryName:  primaryName,
		fallbackName: fallbackName,
		logger:       logger,
	}
}

func verifyDatabaseConnection(db *database.Database) error {
	if db == nil {
		return errors.New("database is nil")
	}
	sqlDB, err := db.DB.DB()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return sqlDB.PingContext(ctx)
}

// -----------------------------------------------------------------------------
// Redis-based lock (for birthday scheduler)
// -----------------------------------------------------------------------------

type redisLock struct {
	client *redis.Client
}

func newRedisLock(redisURL string) *redisLock {
	if strings.TrimSpace(redisURL) == "" {
		return nil
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil
	}
	return &redisLock{client: redis.NewClient(opts)}
}

func (l *redisLock) Acquire(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if l == nil || l.client == nil {
		return true, nil
	}
	return l.client.SetNX(ctx, key, "1", ttl).Result()
}

// -----------------------------------------------------------------------------
// Background jobs
// -----------------------------------------------------------------------------

func startFormCleanup(ctx context.Context, logger *log.Logger, svc service.FormService, interval time.Duration) {
	if svc == nil {
		return
	}
	if interval <= 0 {
		interval = time.Hour
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			count, err := svc.CleanupExpiredForms(time.Now().UTC())
			if err != nil {
				if logger != nil {
					logger.Printf("⚠️ Form cleanup failed: %v", err)
				}
				continue
			}
			if count > 0 && logger != nil {
				logger.Printf("🧹 Cleaned up %d expired forms", count)
			}
		}
	}
}

func startFormReminderScheduler(
	ctx context.Context,
	logger *log.Logger,
	lock *redisLock,
	svc service.FormService,
	interval time.Duration,
	lookAhead time.Duration,
) {
	if svc == nil {
		return
	}
	if interval <= 0 {
		interval = time.Hour
	}
	if lookAhead <= 0 {
		lookAhead = 24 * time.Hour
	}

	run := func() {
		now := time.Now().UTC()
		if lock != nil {
			key := "form_event_reminder:" + now.Format("2006010215")
			ok, err := lock.Acquire(ctx, key, 70*time.Minute)
			if err != nil {
				if logger != nil {
					logger.Printf("⚠️ Form reminder lock failed: %v", err)
				}
				return
			}
			if !ok {
				return
			}
		}

		sent, failed, err := svc.SendEventReminderEmails(now, lookAhead)
		if err != nil {
			if logger != nil {
				logger.Printf("⚠️ Form reminder scheduler failed: %v", err)
			}
			return
		}
		if logger != nil && (sent > 0 || failed > 0) {
			logger.Printf("📩 Form reminders: sent=%d failed=%d", sent, failed)
		}
	}

	run()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func parseHourMinute(raw string) (int, int, error) {
	parts := strings.Split(strings.TrimSpace(raw), ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("time must be HH:MM")
	}
	hour, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("hour must be numeric")
	}
	minute, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("minute must be numeric")
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("time must be valid 24h clock")
	}
	return hour, minute, nil
}

func nextRunAt(now time.Time, hour, minute int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

func startBirthdayScheduler(
	ctx context.Context,
	logger *log.Logger,
	lock *redisLock,
	workforceSvc service.WorkforceService,
	memberSvc service.MemberService,
	leadershipSvc service.LeadershipService,
	tz string,
	sendAt string,
) {
	if workforceSvc == nil && memberSvc == nil && leadershipSvc == nil {
		return
	}

	if strings.TrimSpace(sendAt) == "" {
		sendAt = "09:00"
	}
	hour, minute, err := parseHourMinute(sendAt)
	if err != nil {
		if logger != nil {
			logger.Printf("⚠️ Invalid BIRTHDAY_SCHEDULER_TIME=%q, using 09:00", sendAt)
		}
		hour, minute = 9, 0
	}

	loc := time.UTC
	if strings.TrimSpace(tz) != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		} else if logger != nil {
			logger.Printf("⚠️ Invalid BIRTHDAY_SCHEDULER_TZ=%q, using UTC", tz)
		}
	}

	for {
		now := time.Now().In(loc)
		next := nextRunAt(now, hour, minute)
		timer := time.NewTimer(time.Until(next))

		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		dateKey := next.Format("2006-01-02")
		if lock != nil {
			ok, err := lock.Acquire(ctx, "birthday_send:"+dateKey, 36*time.Hour)
			if err != nil {
				if logger != nil {
					logger.Printf("⚠️ Birthday scheduler lock failed: %v", err)
				}
				continue
			}
			if !ok {
				if logger != nil {
					logger.Printf("ℹ️ Birthday scheduler already ran for %s", dateKey)
				}
				continue
			}
		}

		if workforceSvc != nil {
			result, err := workforceSvc.SendBirthdayGreetings(int(next.Month()), next.Day())
			if err != nil {
				if logger != nil {
					logger.Printf("⚠️ Workforce birthday send failed: %v", err)
				}
			} else if logger != nil {
				logger.Printf("🎂 Workforce birthdays: targeted=%d sent=%d skipped=%d", result.Targeted, result.Sent, result.Skipped)
			}
		}

		if memberSvc != nil {
			result, err := memberSvc.SendBirthdayGreetings(int(next.Month()), next.Day())
			if err != nil {
				if logger != nil {
					logger.Printf("⚠️ Member birthday send failed: %v", err)
				}
			} else if logger != nil {
				logger.Printf("🎉 Member birthdays: targeted=%d sent=%d skipped=%d", result.Targeted, result.Sent, result.Skipped)
			}
		}

		if leadershipSvc != nil {
			result, err := leadershipSvc.SendBirthdayGreetings(int(next.Month()), next.Day())
			if err != nil {
				if logger != nil {
					logger.Printf("⚠️ Leadership birthday send failed: %v", err)
				}
			} else if logger != nil {
				logger.Printf("🎂 Leadership birthdays: targeted=%d sent=%d skipped=%d", result.Targeted, result.Sent, result.Skipped)
			}

			result, err = leadershipSvc.SendAnniversaryGreetings(int(next.Month()), next.Day())
			if err != nil {
				if logger != nil {
					logger.Printf("⚠️ Leadership anniversary send failed: %v", err)
				}
			} else if logger != nil {
				logger.Printf("💍 Leadership anniversaries: targeted=%d sent=%d skipped=%d", result.Targeted, result.Sent, result.Skipped)
			}
		}
	}
}

// -----------------------------------------------------------------------------
// No-op email sender (used when DISABLE_EMAIL / DISABLE_OTP)
// -----------------------------------------------------------------------------

type noopEmailSender struct{}

func (noopEmailSender) SendHTML(string, string, string) error {
	return errors.New("outbound email sending is disabled or not configured")
}

func (noopEmailSender) SendHTMLText(string, string, string, string) error {
	return errors.New("outbound email sending is disabled or not configured")
}

func (noopEmailSender) DisabledReason() string {
	return "outbound email sending is disabled or not configured"
}

// -----------------------------------------------------------------------------
// Router
// -----------------------------------------------------------------------------

func setupRouter(
	cfg *config.Config,
	userRepo repository.UserRepository,
	testimonialHandler *handlers.TestimonialHandler,
	authHandler *handlers.AuthHandler,
	adminHandler *handlers.AdminHandler,
	adminEmailHandler *handlers.AdminEmailHandler,
	uploadHandler *handlers.UploadHandler,
	assetHandler *handlers.AssetHandler,
	eventHandler *handlers.EventHandler,
	adminNotificationHandler *handlers.AdminNotificationHandler,
	approvalRequestHandler *handlers.ApprovalRequestHandler,
	reelHandler *handlers.ReelHandler,
	analyticsHandler *handlers.AnalyticsHandler,
	formHandler *handlers.FormHandler,
	notificationHandler *handlers.NotificationHandler,
	otpHandler *handlers.OTPHandler,
	workforceHandler *handlers.WorkforceHandler,
	leadershipHandler *handlers.LeadershipHandler,
	memberHandler *handlers.MemberHandler,
	emailTemplateHandler *handlers.EmailTemplateHandler,
	emailTemplateRegistryHandler *handlers.EmailTemplateRegistryHandler,
	sermonHandler *handlers.SermonHandler,
	storeHandler *handlers.StoreHandler,
	givingHandler *handlers.GivingHandler,
	siteContentHandler *handlers.SiteContentHandler,
	engagementHandler *handlers.EngagementHandler,
) *gin.Engine {
	router := gin.New()
	secure := strings.TrimSpace(cfg.App.Environment) == "production"

	router.Use(gin.Recovery())
	router.Use(middleware.RequestID())
	router.Use(middleware.Logger(cfg.App.LogLevel))
	router.Use(middleware.SecurityHeaders(secure))
	router.Use(middleware.RequestBodyLimit(cfg.Server.RequestBodyMax))
	router.Use(middleware.CORS(&cfg.CORS))
	router.Use(middleware.RateLimiter(middleware.RateLimiterOptions{
		RequestsPerMinute: cfg.RateLimit.Global.RequestsPerMinute,
		Burst:             cfg.RateLimit.Global.Burst,
		Window:            cfg.RateLimit.Global.Window,
		RedisURL:          cfg.Redis.URL,
		Prefix:            "rl",
		Message:           "Too many requests. Please wait a moment and try again.",
		SkipPathPrefixes:  []string{"/api/v1/auth/login", "/api/v1/auth/otp/"},
	}))

	// Swagger + basic health
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	router.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	router.GET("/forms/:slug", formHandler.ViewPublicFormPage)
	router.GET("/form/:slug", formHandler.RedirectLegacyPublicFormPage)
	router.GET("/reports/forms/:slug", formHandler.ViewPublicFormReport)
	router.GET("/reports/forms/:slug/data", formHandler.GetPublicFormReportData)
	router.GET("/reports/forms/:slug/export.pdf", formHandler.ExportPublicFormReportPDF)

	api := router.Group("/api/v1")

	authGuard := middleware.AuthMiddleware(cfg.JWT.Secret)
	sessionFreshnessGuard := middleware.SessionFreshnessMiddleware(userRepo)
	sessionGuard := middleware.SessionTimeout(cfg.Auth.SessionIdleTimeout, cfg.Auth.RememberedSessionIdleTimeout, secure)
	csrfProtector := middleware.NewCSRFProtector(middleware.CSRFOptions{
		SecretKey:  cfg.Auth.SecretKey,
		Secure:     secure,
		CookieName: cfg.Auth.CSRFCookieName,
		HeaderName: cfg.Auth.CSRFHeaderName,
		CookieTTL:  cfg.Auth.CSRFCookieTTL,
	})

	// AUTH
	auth := api.Group("/auth")
	auth.Use(middleware.DeviceFingerprint(secure))
	auth.Use(middleware.NoStore())
	loginRateLimiter := middleware.RateLimiter(middleware.RateLimiterOptions{
		RequestsPerMinute: cfg.RateLimit.Auth.RequestsPerMinute,
		Burst:             cfg.RateLimit.Auth.Burst,
		Window:            cfg.RateLimit.Auth.Window,
		RedisURL:          cfg.Redis.URL,
		Prefix:            "rl:login",
		Message:           "Too many authentication attempts. Please wait a moment and try again.",
	})

	auth.POST("/login", loginRateLimiter, authHandler.Login)
	// Backwards-compatible aliases for older admin clients
	auth.POST("/login/verify-otp", loginRateLimiter, authHandler.VerifyLoginOTP)
	auth.POST("/login/resend-otp", loginRateLimiter, authHandler.ResendLoginOTP)
	auth.POST("/register", loginRateLimiter, authHandler.Register)
	auth.POST("/password-reset/request", loginRateLimiter, authHandler.RequestPasswordReset)
	auth.POST("/password-reset/confirm", loginRateLimiter, authHandler.ConfirmPasswordReset)
	auth.POST("/otp/verify", loginRateLimiter, authHandler.VerifyLoginOTP)
	auth.POST("/otp/resend", loginRateLimiter, authHandler.ResendLoginOTP)
	auth.GET("/oauth/google/start", authHandler.StartGoogleOAuth)
	auth.GET("/oauth/google/callback", authHandler.HandleGoogleOAuthCallback)
	auth.GET("/me", middleware.OptionalAuthMiddleware(cfg.JWT.Secret), authHandler.GetCurrentUser)

	authProtected := auth.Group("")
	authProtected.Use(authGuard, sessionFreshnessGuard, sessionGuard, csrfProtector.Middleware(), middleware.AuditLogger("auth"))
	authProtected.GET("/csrf-token", authHandler.GetCSRFToken)
	authProtected.PATCH("/profile", authHandler.UpdateProfile)
	authProtected.POST("/change-password", authHandler.ChangePassword)
	authProtected.DELETE("/account", authHandler.DeleteAccount)
	authProtected.POST("/clear-data", authHandler.ClearData)
	authProtected.POST("/refresh", authHandler.RefreshToken)
	authProtected.POST("/logout", authHandler.Logout)
	authProtected.GET("/mfa", authHandler.GetMFASecurityProfile)
	authProtected.POST("/mfa/totp/setup", authHandler.BeginTOTPSetup)
	authProtected.POST("/mfa/totp/enable", authHandler.EnableTOTP)
	authProtected.POST("/mfa/totp/disable", authHandler.DisableTOTP)
	authProtected.PATCH("/mfa/method", authHandler.SetPreferredMFAMethod)

	// OTP
	otpRateLimiter := middleware.RateLimiter(middleware.RateLimiterOptions{
		RequestsPerMinute: cfg.RateLimit.Auth.RequestsPerMinute,
		Burst:             cfg.RateLimit.Auth.Burst,
		Window:            cfg.RateLimit.Auth.Window,
		RedisURL:          cfg.Redis.URL,
		Prefix:            "rl:otp",
		Message:           "Too many OTP requests. Please wait before trying again.",
	})
	api.POST("/otp/send", middleware.NoStore(), otpRateLimiter, otpHandler.SendOTP)
	api.POST("/otp/verify", middleware.NoStore(), otpRateLimiter, otpHandler.VerifyOTP)

	// Public testimonials
	api.GET("/testimonials", testimonialHandler.GetPaginatedTestimonials)
	api.GET("/testimonials/all", testimonialHandler.GetAllTestimonials)
	api.GET("/testimonials/:id", testimonialHandler.GetTestimonialByID)
	api.POST("/testimonials", testimonialHandler.CreateTestimonial)

	// Public events/reels
	api.GET("/events", eventHandler.List)
	api.GET("/events/:id", eventHandler.Get)
	api.GET("/reels", reelHandler.List)
	api.GET("/sermons", sermonHandler.List)
	api.POST("/analytics/events", analyticsHandler.IngestEvents)

	// Notifications (newsletter-style)
	api.POST("/notifications/subscribe", notificationHandler.Subscribe)
	api.GET("/notifications/subscribe", notificationHandler.SubscribeByLink)
	api.POST("/notifications/unsubscribe", notificationHandler.Unsubscribe)
	api.GET("/notifications/unsubscribe", notificationHandler.UnsubscribeByLink)

	// Public store APIs (backend-driven)
	api.GET("/store/products", storeHandler.ListProducts)
	api.POST("/store/orders", storeHandler.CreateOrder)
	api.GET("/store/orders/:orderId", storeHandler.GetOrder)
	api.GET("/giving/options", givingHandler.ListOptions)
	api.POST("/giving/intents", engagementHandler.CreateGivingIntent)
	api.POST("/pastoral-care/requests", engagementHandler.CreatePastoralCareRequest)
	api.POST("/contact/messages", engagementHandler.CreateContactMessage)
	api.GET("/content/homepage-ad", siteContentHandler.GetHomepageAd)
	api.GET("/content/confession-popup", siteContentHandler.GetConfessionPopup)

	// Public forms
	api.GET("/forms/:slug", formHandler.GetPublicForm)
	api.POST("/forms/:slug/submissions", formHandler.SubmitPublicForm)
	api.GET("/forms/:slug/report", formHandler.ViewPublicFormReport)
	api.GET("/forms/:slug/report/data", formHandler.GetPublicFormReportData)
	api.GET("/forms/:slug/report/export.pdf", formHandler.ExportPublicFormReportPDF)
	api.GET("/forms/:slug/calendar/confirm", formHandler.ConfirmCalendarOptIn)
	api.GET("/forms/:slug/calendar.ics", formHandler.DownloadCalendarICS)

	// Workforce public apply
	api.POST("/workforce/apply", workforceHandler.Apply)
	api.POST("/workforce/serving/register", workforceHandler.ApplyServing)

	// Leadership public
	api.GET("/leadership", leadershipHandler.ListPublic)
	api.POST("/leadership/apply", leadershipHandler.Apply)
	api.POST("/leadership/upload-image", leadershipHandler.UploadImage)
	api.POST("/leadership/upload", leadershipHandler.UploadImage)

	// Universal public uploads for forms and future public upload flows.
	api.POST("/uploads", uploadHandler.UploadFile)
	api.POST("/uploads/files", uploadHandler.UploadFile)
	api.POST("/uploads/images", uploadHandler.UploadImage)

	// ADMIN
	admin := api.Group("/admin")
	admin.Use(
		authGuard,
		sessionFreshnessGuard,
		sessionGuard,
		middleware.RoleMiddleware("admin"),
		middleware.RequireAdminMFA(userRepo),
		middleware.RequirePermission(middleware.PermissionAdminAccess),
		csrfProtector.Middleware(),
		middleware.AuditLogger("admin"),
	)

	admin.GET("/dashboard", middleware.RequirePermission(middleware.PermissionAdminRead), adminHandler.GetDashboardStats)
	admin.GET("/security/overview", middleware.RequirePermission(middleware.PermissionSecurityRead), adminHandler.GetSecurityOverview)
	admin.GET("/testimonials/pending", middleware.RequirePermission(middleware.PermissionAdminRead), adminHandler.GetPendingTestimonials)

	admin.GET("/users", middleware.RequirePermission(middleware.PermissionUsersManage), adminHandler.ListUsers)
	admin.GET("/users/:id", middleware.RequirePermission(middleware.PermissionUsersManage), adminHandler.GetUserByID)
	admin.POST("/users", middleware.RequirePermission(middleware.PermissionUsersManage), adminHandler.CreateUser)
	admin.PATCH("/users/:id", middleware.RequirePermission(middleware.PermissionUsersManage), adminHandler.UpdateUser)
	admin.DELETE("/users/:id", middleware.RequirePermission(middleware.PermissionUsersManage), adminHandler.DeleteUser)

	admin.GET("/analytics", middleware.RequirePermission(middleware.PermissionAnalyticsRead), analyticsHandler.GetAdminAnalytics)
	admin.GET("/analytics/insights", middleware.RequirePermission(middleware.PermissionAnalyticsRead), analyticsHandler.GetDecisionInsights)
	admin.GET("/content/homepage-ad", middleware.RequirePermission(middleware.PermissionContentManage), siteContentHandler.GetAdminHomepageAd)
	admin.PUT("/content/homepage-ad", middleware.RequirePermission(middleware.PermissionContentManage), siteContentHandler.UpdateAdminHomepageAd)
	admin.GET("/content/confession-popup", middleware.RequirePermission(middleware.PermissionContentManage), siteContentHandler.GetAdminConfessionPopup)
	admin.PUT("/content/confession-popup", middleware.RequirePermission(middleware.PermissionContentManage), siteContentHandler.UpdateAdminConfessionPopup)
	admin.GET("/pastoral-care/requests", middleware.RequirePermission(middleware.PermissionEngagementRead), engagementHandler.ListPastoralCareRequests)
	admin.GET("/giving/intents", middleware.RequirePermission(middleware.PermissionEngagementRead), engagementHandler.ListGivingIntents)
	admin.GET("/contact/messages", middleware.RequirePermission(middleware.PermissionEngagementRead), engagementHandler.ListContactMessages)

	// Forms
	// Forms
	admin.GET("/forms", middleware.RequirePermission(middleware.PermissionFormsRead), formHandler.ListAdminForms)
	admin.GET("/forms/:id", middleware.RequirePermission(middleware.PermissionFormsRead), formHandler.GetAdminForm)
	admin.POST("/forms", middleware.RequirePermission(middleware.PermissionFormsManage), formHandler.CreateAdminForm)
	admin.PUT("/forms/:id", middleware.RequirePermission(middleware.PermissionFormsManage), formHandler.UpdateAdminForm)
	admin.DELETE("/forms/:id", middleware.RequirePermission(middleware.PermissionFormsManage), formHandler.DeleteAdminForm)
	admin.POST(
		"/forms/:id/banner",
		middleware.RequirePermission(middleware.PermissionFormsManage),
		middleware.RequirePermission(middleware.PermissionUploadsManage),
		formHandler.UploadAdminFormBanner,
	)
	admin.POST("/forms/:id/publish", middleware.RequirePermission(middleware.PermissionFormsManage), formHandler.PublishAdminForm)
	admin.GET("/forms/:id/report-link", middleware.RequirePermission(middleware.PermissionFormsExport), formHandler.GetAdminFormReportLink)
	admin.POST("/forms/:id/report-link", middleware.RequirePermission(middleware.PermissionFormsExport), formHandler.GetAdminFormReportLink)
	admin.GET("/forms/:id/campaigns/history", middleware.RequirePermission(middleware.PermissionFormsRead), formHandler.ListAdminFormCampaignHistory)
	admin.POST("/forms/:id/campaigns/send", middleware.RequirePermission(middleware.PermissionFormsCampaign), formHandler.SendAdminFormCampaign)

	admin.GET("/forms/:id/submissions", middleware.RequirePermission(middleware.PermissionFormsRead), formHandler.ListAdminSubmissions)
	admin.GET("/forms/:id/submissions/export.pdf", middleware.RequirePermission(middleware.PermissionFormsExport), formHandler.ExportAdminSubmissionsPDF) // ✅ ADD THIS
	admin.GET("/forms/:id/submissions/stats", middleware.RequirePermission(middleware.PermissionFormsRead), formHandler.GetFormSubmissionStats)

	admin.GET("/forms/stats", middleware.RequirePermission(middleware.PermissionFormsRead), formHandler.GetFormStats)

	// Notifications (admin)
	admin.GET("/notifications/subscribers", middleware.RequirePermission(middleware.PermissionNotificationsManage), notificationHandler.ListSubscribers)
	admin.GET("/notifications/subscribers/summary", middleware.RequirePermission(middleware.PermissionNotificationsManage), notificationHandler.GetSubscriberSummary)
	admin.POST("/notifications/send", middleware.RequirePermission(middleware.PermissionNotificationsManage), notificationHandler.SendNotification)
	admin.GET("/notifications/inbox", middleware.RequirePermission(middleware.PermissionNotificationsManage), adminNotificationHandler.List)
	admin.PATCH("/notifications/:id/read", middleware.RequirePermission(middleware.PermissionNotificationsManage), adminNotificationHandler.MarkRead)
	admin.POST("/notifications/read-all", middleware.RequirePermission(middleware.PermissionNotificationsManage), adminNotificationHandler.MarkAllRead)

	// Approval requests
	admin.GET("/requests", middleware.RequirePermission(middleware.PermissionApprovalsRead), approvalRequestHandler.List)
	admin.GET("/requests/timeline", middleware.RequirePermission(middleware.PermissionApprovalsRead), approvalRequestHandler.Timeline)

	// Email templates
	admin.POST("/email/templates/send", middleware.RequirePermission(middleware.PermissionEmailManage), emailTemplateHandler.SendTemplate)
	admin.GET("/email/templates", middleware.RequirePermission(middleware.PermissionEmailManage), emailTemplateRegistryHandler.List)
	admin.POST("/email/templates", middleware.RequirePermission(middleware.PermissionEmailManage), emailTemplateRegistryHandler.Create)
	admin.GET("/email/templates/:id", middleware.RequirePermission(middleware.PermissionEmailManage), emailTemplateRegistryHandler.Get)
	admin.PUT("/email/templates/:id", middleware.RequirePermission(middleware.PermissionEmailManage), emailTemplateRegistryHandler.Update)
	admin.POST("/email/templates/:id/activate", middleware.RequirePermission(middleware.PermissionEmailManage), emailTemplateRegistryHandler.Activate)
	admin.GET("/email/marketing/summary", middleware.RequirePermission(middleware.PermissionEmailManage), adminEmailHandler.GetMarketingSummary)
	admin.GET("/email/marketing/forms", middleware.RequirePermission(middleware.PermissionEmailManage), adminEmailHandler.ListAudienceForms)
	admin.GET("/email/marketing/audience/preview", middleware.RequirePermission(middleware.PermissionEmailManage), adminEmailHandler.PreviewAudience)
	admin.GET("/email/compose/history", middleware.RequirePermission(middleware.PermissionEmailManage), adminEmailHandler.ListComposeHistory)
	admin.POST("/email/compose/send", middleware.RequirePermission(middleware.PermissionEmailManage), adminEmailHandler.SendComposeEmail)

	// Uploads
	admin.POST("/uploads/images", middleware.RequirePermission(middleware.PermissionUploadsManage), uploadHandler.UploadImage)
	admin.POST("/uploads/files", middleware.RequirePermission(middleware.PermissionUploadsManage), uploadHandler.UploadFile)
	admin.POST("/uploads", middleware.RequirePermission(middleware.PermissionUploadsManage), uploadHandler.UploadFile)
	admin.POST("/uploads/presign", middleware.RequirePermission(middleware.PermissionUploadsManage), assetHandler.Presign)
	admin.POST("/uploads/:id/complete", middleware.RequirePermission(middleware.PermissionUploadsManage), assetHandler.Complete)
	admin.GET("/uploads/:id", middleware.RequirePermission(middleware.PermissionUploadsManage), assetHandler.Get)
	admin.GET("/uploads", middleware.RequirePermission(middleware.PermissionUploadsManage), assetHandler.List)

	// Events
	admin.GET("/events", middleware.RequirePermission(middleware.PermissionEventsManage), eventHandler.List)
	admin.POST("/events", middleware.RequirePermission(middleware.PermissionEventsManage), eventHandler.Create)
	admin.PUT("/events/:id", middleware.RequirePermission(middleware.PermissionEventsManage), eventHandler.Update)
	admin.DELETE("/events/:id", middleware.RequirePermission(middleware.PermissionEventsManage), eventHandler.Delete)
	admin.POST("/events/:id/image", middleware.RequirePermission(middleware.PermissionEventsManage), eventHandler.UploadImage)
	admin.POST("/events/:id/banner", middleware.RequirePermission(middleware.PermissionEventsManage), eventHandler.UploadBanner)

	// Reels
	admin.GET("/reels", middleware.RequirePermission(middleware.PermissionReelsManage), reelHandler.List)
	admin.POST("/reels", middleware.RequirePermission(middleware.PermissionReelsManage), reelHandler.Create)
	admin.DELETE("/reels/:id", middleware.RequirePermission(middleware.PermissionReelsManage), reelHandler.Delete)

	// Workforce admin
	admin.GET("/workforce", middleware.RequirePermission(middleware.PermissionWorkforceManage), workforceHandler.List)
	admin.POST("/workforce", middleware.RequirePermission(middleware.PermissionWorkforceManage), workforceHandler.Create)
	admin.PUT("/workforce/:id", middleware.RequirePermission(middleware.PermissionWorkforceManage), workforceHandler.Update)
	admin.GET("/workforce/stats", middleware.RequirePermission(middleware.PermissionWorkforceManage), workforceHandler.Stats)
	admin.GET("/workforce/birthdays/stats", middleware.RequirePermission(middleware.PermissionWorkforceManage), workforceHandler.BirthdayStats)
	admin.GET("/workforce/birthdays/month/:month", middleware.RequirePermission(middleware.PermissionWorkforceManage), workforceHandler.BirthdaysByMonth)
	admin.GET("/workforce/birthdays/today", middleware.RequirePermission(middleware.PermissionWorkforceManage), workforceHandler.BirthdaysToday)
	admin.POST("/workforce/birthdays/send-today", middleware.RequirePermission(middleware.PermissionWorkforceManage), workforceHandler.SendBirthdaysToday)

	// Members admin
	admin.GET("/members", middleware.RequirePermission(middleware.PermissionMembersManage), memberHandler.List)
	admin.POST("/members", middleware.RequirePermission(middleware.PermissionMembersManage), memberHandler.Create)
	admin.PUT("/members/:id", middleware.RequirePermission(middleware.PermissionMembersManage), memberHandler.Update)
	admin.DELETE("/members/:id", middleware.RequirePermission(middleware.PermissionMembersManage), memberHandler.Delete)
	admin.GET("/members/birthdays/stats", middleware.RequirePermission(middleware.PermissionMembersManage), memberHandler.BirthdayStats)
	admin.GET("/members/birthdays/month/:month", middleware.RequirePermission(middleware.PermissionMembersManage), memberHandler.BirthdaysByMonth)
	admin.GET("/members/birthdays/today", middleware.RequirePermission(middleware.PermissionMembersManage), memberHandler.BirthdaysToday)
	admin.POST("/members/birthdays/send-today", middleware.RequirePermission(middleware.PermissionMembersManage), memberHandler.SendBirthdaysToday)

	// Store admin
	admin.GET("/store/products", middleware.RequirePermission(middleware.PermissionStoreManage), storeHandler.ListProductsAdmin)
	admin.POST("/store/products", middleware.RequirePermission(middleware.PermissionStoreManage), storeHandler.CreateProduct)
	admin.PUT("/store/products/:id", middleware.RequirePermission(middleware.PermissionStoreManage), storeHandler.UpdateProduct)
	admin.PATCH("/store/products/:id/stock", middleware.RequirePermission(middleware.PermissionStoreManage), storeHandler.UpdateProductStock)
	admin.PATCH("/store/products/:id/active", middleware.RequirePermission(middleware.PermissionStoreManage), storeHandler.UpdateProductActive)
	admin.GET("/store/orders", middleware.RequirePermission(middleware.PermissionStoreManage), storeHandler.ListOrdersAdmin)
	admin.PATCH("/store/orders/:orderId/status", middleware.RequirePermission(middleware.PermissionStoreManage), storeHandler.UpdateOrderStatus)
	admin.POST("/members/notify", middleware.RequirePermission(middleware.PermissionMembersManage), memberHandler.SendAnnouncement)

	// Leadership admin
	admin.GET("/leadership", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.List)
	admin.POST("/leadership", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.Create)
	admin.PUT("/leadership/:id", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.Update)
	admin.DELETE("/leadership/:id", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.Delete)
	admin.GET("/leadership/birthdays/stats", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.BirthdayStats)
	admin.GET("/leadership/birthdays/month/:month", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.BirthdaysByMonth)
	admin.GET("/leadership/birthdays/today", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.BirthdaysToday)
	admin.POST("/leadership/birthdays/send-today", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.SendBirthdaysToday)
	admin.GET("/leadership/anniversaries/stats", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.AnniversaryStats)
	admin.GET("/leadership/anniversaries/month/:month", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.AnniversariesByMonth)
	admin.GET("/leadership/anniversaries/today", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.AnniversariesToday)
	admin.POST("/leadership/anniversaries/send-today", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.SendAnniversariesToday)

	// Super-admin
	superAdmin := admin.Group("")
	superAdmin.Use(middleware.RoleMiddleware("super_admin"))
	superAdmin.POST("/users/:id/approve", middleware.RequirePermission(middleware.PermissionUsersManage), adminHandler.ApproveUser)
	superAdmin.POST("/workforce/:id/approve", middleware.RequirePermission(middleware.PermissionWorkforceManage), workforceHandler.Approve)
	superAdmin.POST("/leadership/:id/approve", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.Approve)
	superAdmin.POST("/leadership/:id/decline", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.Decline)
	superAdmin.PATCH("/testimonials/:id/approve", middleware.RequirePermission(middleware.PermissionAdminWrite), testimonialHandler.ApproveTestimonial)
	superAdmin.DELETE("/testimonials/:id", middleware.RequirePermission(middleware.PermissionAdminWrite), testimonialHandler.DeleteTestimonial)
	superAdmin.PATCH("/events/:id/approve", middleware.RequirePermission(middleware.PermissionEventsManage), eventHandler.Approve)

	return router
}

// -----------------------------------------------------------------------------
// main
// -----------------------------------------------------------------------------

func main() {
	logger := log.New(os.Stdout, "🚀 ", log.Ldate|log.Ltime|log.Lshortfile)

	logger.Println("Loading configuration...")
	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("❌ Failed to load configuration: %v", err)
	}

	validation.Init()

	env := strings.ToLower(strings.TrimSpace(cfg.App.Environment))
	if env == "" {
		env = "development"
	}
	cfg.App.Environment = env

	disableOTP := isTrueEnv("DISABLE_OTP")
	disableLoginOTP := isTrueEnv("DISABLE_LOGIN_OTP")
	disableEmail := isTrueEnv("DISABLE_EMAIL") || disableOTP
	if disableOTP {
		logger.Println("⚠️ DISABLE_OTP=true: OTP verification is disabled for login/password reset.")
	}
	if disableLoginOTP {
		logger.Println("⚠️ DISABLE_LOGIN_OTP=true: OTP challenges are disabled for login.")
	}
	if disableEmail {
		logger.Println("⚠️ DISABLE_EMAIL=true: outbound email sending is disabled.")
	}

	// Gin mode
	if cfg.App.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	} else if strings.TrimSpace(cfg.Server.GinMode) != "" {
		gin.SetMode(cfg.Server.GinMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	ensureCORSDefaults(cfg)

	// -------------------------------------------------------------------------
	// Asset uploader (S3-compatible)
	// -------------------------------------------------------------------------
	var assetUploader service.AssetUploader
	if uploader, err := service.NewS3UploaderFromEnv(); err != nil {
		logger.Printf("⚠️ Storage uploader not initialized: %v", err)
	} else if uploader != nil {
		assetUploader = uploader
		logger.Printf("✅ Storage uploader initialized (%s)", uploader.StorageSummary())
	}

	// Database
	db, err := database.NewDatabase(&cfg.Database, cfg.App.Environment)
	if err != nil {
		logger.Fatalf("❌ Failed to connect to database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Printf("⚠️ Error closing database: %v", err)
		}
		logger.Println("✅ Database connection closed")
	}()

	if err := verifyDatabaseConnection(db); err != nil {
		logger.Fatalf("❌ Database connection failed: %v", err)
	}

	// Migration-only mode
	if isTrueEnv("RUN_AUTOMIGRATE") {
		logger.Println("✅ RUN_AUTOMIGRATE=true: migrations executed. Exiting without starting server.")
		return
	}

	// -------------------------------------------------------------------------
	// Repositories
	// -------------------------------------------------------------------------
	testimonialRepo := repository.NewTestimonialRepository(db)
	userRepo := repository.NewUserRepository(db)
	adminRepo := repository.NewAdminRepository(db)
	eventRepo := repository.NewEventRepository(db)
	reelRepo := repository.NewReelRepository(db)
	formRepo := repository.NewFormRepository(db)
	formCalendarReminderRepo := repository.NewFormCalendarReminderRepository(db)
	assetRepo := repository.NewAssetRepository(db)
	adminEmailDeliveryRepo := repository.NewAdminEmailDeliveryRepository(db)
	emailTemplateRepo := repository.NewEmailTemplateRepository(db)
	subscriberRepo := repository.NewSubscriberRepository(db)
	notificationRepo := repository.NewNotificationRepository(db)
	approvalRequestRepo := repository.NewApprovalRequestRepository(db)
	adminNotificationRepo := repository.NewAdminNotificationRepository(db)
	ticketSequenceRepo := repository.NewTicketSequenceRepository(db)
	registrationSequenceRepo := repository.NewRegistrationSequenceRepository(db)
	otpRepo := repository.NewOTPRepository(db)
	workforceRepo := repository.NewWorkforceRepository(db)
	leadershipRepo := repository.NewLeadershipRepository(db)
	memberRepo := repository.NewMemberRepository(db)
	securityEventRepo := repository.NewSecurityEventRepository(db)
	trustedDeviceRepo := repository.NewTrustedDeviceRepository(db)
	storeRepo := repository.NewStoreRepository(db)

	// -------------------------------------------------------------------------
	// Redis cache client (optional)
	// -------------------------------------------------------------------------
	var redisCache *cache.RedisClient
	if strings.TrimSpace(cfg.Redis.URL) != "" {
		redisCache, err = cache.NewRedisClient(cache.Config{
			URL:          cfg.Redis.URL,
			PoolSize:     cfg.Redis.PoolSize,
			DialTimeout:  cfg.Redis.DialTimeout,
			ReadTimeout:  cfg.Redis.ReadTimeout,
			WriteTimeout: cfg.Redis.WriteTimeout,
			PoolTimeout:  cfg.Redis.PoolTimeout,
		})
		if err != nil {
			logger.Printf("⚠️ Redis cache not initialized: %v", err)
		} else {
			defer func() {
				if cerr := redisCache.Close(); cerr != nil {
					logger.Printf("⚠️ Error closing redis cache: %v", cerr)
				}
			}()
			logger.Println("✅ Redis cache initialized")
		}
	}

	// -------------------------------------------------------------------------
	// Email sender (Brevo / SES)
	// -------------------------------------------------------------------------
	var emailSender service.EmailSender
	if disableEmail {
		emailSender = noopEmailSender{}
		logger.Println("⚠️ Email sender disabled (DISABLE_EMAIL/DISABLE_OTP)")
	} else {
		emailSender = initEmailSender(cfg, logger)
		emailSender = observedEmailSender{inner: emailSender, logger: logger}
	}

	templateAssetBaseURL := strings.TrimRight(strings.TrimSpace(cfg.App.EmailTemplateAssetBaseURL), "/")
	if templateAssetBaseURL == "" {
		base := strings.TrimRight(firstNonEmptyEnv("S3_PUBLIC_BASE_URL"), "/")
		path := strings.Trim(firstNonEmptyEnv("S3_EMAIL_TEMPLATE_PATH"), "/")
		if base != "" && path != "" {
			templateAssetBaseURL = base + "/" + path
		} else if base != "" {
			templateAssetBaseURL = base
		}
	}

	branding := email.Branding{
		AppName:              cfg.App.Name,
		LogoURL:              cfg.App.LogoURL,
		PublicURL:            cfg.App.PublicURL,
		FrontendURL:          cfg.App.FrontendURL,
		SupportEmail:         cfg.App.SupportEmail,
		PastorName:           cfg.App.PastorName,
		AdminPortalURL:       cfg.App.AdminPortalURL,
		TemplateAssetBaseURL: templateAssetBaseURL,
	}

	// -------------------------------------------------------------------------
	// Services
	// -------------------------------------------------------------------------
	adminNotificationService := service.NewAdminNotificationService(
		adminNotificationRepo,
		userRepo,
		emailSender,
		branding,
	)

	approvalService := service.NewApprovalService(
		approvalRequestRepo,
		ticketSequenceRepo,
	)

	testimonialService := service.NewTestimonialService(
		testimonialRepo,
		assetRepo,
		assetUploader,
		approvalService,
		adminNotificationService,
	)

	otpService := service.NewOTPService(otpRepo, emailSender, branding, userRepo)

	securityService := service.NewSecurityService(
		securityEventRepo,
		trustedDeviceRepo,
		emailSender,
		branding,
		func() string {
			if strings.TrimSpace(cfg.App.AdminPortalURL) != "" {
				return cfg.App.AdminPortalURL
			}
			return cfg.App.FrontendURL
		}(),
	)

	authService := service.NewAuthService(
		userRepo,
		otpService,
		emailSender,
		branding,
		securityService,
		trustedDeviceRepo,
		disableOTP,
		disableLoginOTP,
		approvalService,
		adminNotificationService,
		cfg.Auth.MFAIssuer,
		cfg.Auth.SecretKey,
	)

	adminService := service.NewAdminService(
		adminRepo,
		testimonialRepo,
		userRepo,
		approvalService,
		adminNotificationService,
		emailSender,
		branding,
	)

	assetService := service.NewAssetService(assetRepo, assetUploader)
	adminEmailService := service.NewAdminEmailService(formRepo, emailTemplateRepo, adminEmailDeliveryRepo, emailSender, branding)
	emailTemplateRegistryService := service.NewEmailTemplateRegistryService(emailTemplateRepo)

	workforceService := service.NewWorkforceService(workforceRepo, emailSender, branding)
	leadershipService := service.NewLeadershipService(leadershipRepo, adminNotificationService, emailSender, branding)
	memberService := service.NewMemberService(memberRepo, eventRepo, emailSender, branding)

	publicBaseURL := strings.TrimRight(strings.TrimSpace(cfg.App.PublicURL), "/")
	formService := service.NewFormService(
		formRepo,
		eventRepo,
		registrationSequenceRepo,
		formCalendarReminderRepo,
		emailTemplateRepo,
		workforceService,
		memberService,
		leadershipService,
		testimonialService,
		emailSender,
		branding,
		assetUploader,
		publicBaseURL,
	)

	notificationService := service.NewNotificationService(
		subscriberRepo,
		notificationRepo,
		eventRepo,
		emailSender,
		branding,
	)
	storeService := service.NewStoreService(storeRepo)
	sermonService := service.NewSermonService()

	emailTemplateService := service.NewEmailTemplateService(emailSender, branding)

	// -------------------------------------------------------------------------
	// Handlers
	// -------------------------------------------------------------------------
	testimonialHandler := handlers.NewTestimonialHandler(testimonialService, userRepo)
	authHandler := handlers.NewAuthHandler(authService, handlers.AuthHandlerOptions{
		JWTSecret:                    cfg.JWT.Secret,
		Secure:                       strings.TrimSpace(cfg.App.Environment) == "production",
		AccessTokenTTL:               cfg.JWT.Expiration,
		RememberMeTTL:                cfg.Auth.RememberMeTTL,
		SessionIdleTimeout:           cfg.Auth.SessionIdleTimeout,
		RememberedSessionIdleTimeout: cfg.Auth.RememberedSessionIdleTimeout,
		PostLoginRedirectURL:         cfg.App.AdminPortalURL,
		AuthSecretKey:                cfg.Auth.SecretKey,
		GoogleClientID:               cfg.Auth.GoogleClientID,
		GoogleClientSecret:           cfg.Auth.GoogleClientSecret,
		GoogleRedirectURL:            cfg.Auth.GoogleRedirectURL,
		GoogleHostedDomain:           cfg.Auth.GoogleHostedDomain,
	})
	adminHandler := handlers.NewAdminHandler(adminService)
	adminEmailHandler := handlers.NewAdminEmailHandler(adminEmailService)
	uploadHandler := handlers.NewUploadHandler(assetUploader, assetService)
	assetHandler := handlers.NewAssetHandler(assetService)
	eventHandler := handlers.NewEventHandler(
		eventRepo,
		assetUploader,
		userRepo,
		approvalService,
		adminNotificationService,
	)
	approvalRequestHandler := handlers.NewApprovalRequestHandler(approvalService, approvalRequestRepo)
	adminNotificationHandler := handlers.NewAdminNotificationHandler(adminNotificationService)
	reelHandler := handlers.NewReelHandler(reelRepo)
	analyticsHandler := handlers.NewAnalyticsHandler(db, redisCache)
	formHandler := handlers.NewFormHandler(formService, assetUploader)
	notificationHandler := handlers.NewNotificationHandler(notificationService)
	otpHandler := handlers.NewOTPHandler(otpService)
	workforceHandler := handlers.NewWorkforceHandler(
		workforceService,
		adminNotificationService,
		emailSender,
		emailTemplateRepo,
		branding,
	)
	leadershipHandler := handlers.NewLeadershipHandler(leadershipService, assetUploader)
	memberHandler := handlers.NewMemberHandler(memberService)
	emailTemplateHandler := handlers.NewEmailTemplateHandler(emailTemplateService)
	emailTemplateRegistryHandler := handlers.NewEmailTemplateRegistryHandler(emailTemplateRegistryService)
	sermonHandler := handlers.NewSermonHandler(sermonService)
	storeHandler := handlers.NewStoreHandler(storeService)
	givingHandler := handlers.NewGivingHandler()
	siteContentHandler := handlers.NewSiteContentHandler(db)
	engagementHandler := handlers.NewEngagementHandler(
		db,
		adminNotificationService,
		emailSender,
		emailTemplateRepo,
		branding,
	)

	// -------------------------------------------------------------------------
	// Background jobs
	// -------------------------------------------------------------------------
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	defer cleanupCancel()

	go startFormCleanup(cleanupCtx, logger, formService, cfg.App.FormCleanupInterval)
	go startFormReminderScheduler(cleanupCtx, logger, newRedisLock(cfg.Redis.URL), formService, time.Hour, 24*time.Hour)

	schedulerEnabled := isTrueEnv("BIRTHDAY_SCHEDULER_ENABLED")
	if schedulerEnabled {
		lock := newRedisLock(cfg.Redis.URL)
		tz := strings.TrimSpace(os.Getenv("BIRTHDAY_SCHEDULER_TZ"))
		sendAt := strings.TrimSpace(os.Getenv("BIRTHDAY_SCHEDULER_TIME"))
		go startBirthdayScheduler(cleanupCtx, logger, lock, workforceService, memberService, leadershipService, tz, sendAt)
	}
	if isTrueEnv("BIRTHDAY_SCHEDULER_ONLY") {
		if !schedulerEnabled {
			logger.Println("⚠️ BIRTHDAY_SCHEDULER_ONLY=true but BIRTHDAY_SCHEDULER_ENABLED=false")
		}
		logger.Println("🎂 Birthday scheduler worker mode enabled (API server disabled)")
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		sig := <-quit
		logger.Printf("🛑 Received signal: %v", sig)
		cleanupCancel()
		return
	}

	// -------------------------------------------------------------------------
	// HTTP server
	// -------------------------------------------------------------------------
	router := setupRouter(
		cfg,
		userRepo,
		testimonialHandler,
		authHandler,
		adminHandler,
		adminEmailHandler,
		uploadHandler,
		assetHandler,
		eventHandler,
		adminNotificationHandler,
		approvalRequestHandler,
		reelHandler,
		analyticsHandler,
		formHandler,
		notificationHandler,
		otpHandler,
		workforceHandler,
		leadershipHandler,
		memberHandler,
		emailTemplateHandler,
		emailTemplateRegistryHandler,
		sermonHandler,
		storeHandler,
		givingHandler,
		siteContentHandler,
		engagementHandler,
	)

	if err := router.SetTrustedProxies(cfg.Server.TrustedProxies); err != nil {
		logger.Fatalf("❌ Invalid SERVER_TRUSTED_PROXIES: %v", err)
	}
	logger.Printf("✅ Trusted proxies configured: %v", cfg.Server.TrustedProxies)

	server := &http.Server{
		Addr:              ":" + cfg.Server.Port,
		Handler:           router,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		MaxHeaderBytes:    cfg.Server.MaxHeaderBytes,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Graceful shutdown
	shutdownErr := make(chan error, 1)
	go func() {
		host := "localhost"
		if cfg.App.Environment == "production" {
			host = "0.0.0.0"
		}
		logger.Printf("✅ Server starting on http://%s:%s", host, cfg.Server.Port)

		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			shutdownErr <- err
			return
		}
		shutdownErr <- nil
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		logger.Printf("🛑 Received signal: %v", sig)
	case err := <-shutdownErr:
		if err != nil {
			logger.Fatalf("❌ Server failed: %v", err)
		}
		logger.Println("ℹ️ Server exited.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger.Println("🔄 Shutting down server gracefully...")
	server.SetKeepAlivesEnabled(false)
	cleanupCancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Fatalf("❌ Server forced to shutdown: %v", err)
	}
	logger.Println("👋 Server exited gracefully")
}
