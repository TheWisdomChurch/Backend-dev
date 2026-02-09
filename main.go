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

func ensureCORSDefaults(cfg *config.Config) {
	if cfg == nil {
		return
	}

	// If CORS_ALLOW_ORIGIN is explicitly set, respect it.
	if v, ok := os.LookupEnv("CORS_ALLOW_ORIGIN"); ok && strings.TrimSpace(v) != "" {
		return
	}

	// Otherwise, ensure app URLs are allowed (useful for production).
	existing := make(map[string]struct{}, len(cfg.CORS.AllowedOrigins))
	for _, o := range cfg.CORS.AllowedOrigins {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		existing[o] = struct{}{}
	}

	candidates := []string{
		strings.TrimSpace(cfg.App.FrontendURL),
		strings.TrimSpace(cfg.App.AdminPortalURL),
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

type chainedEmailSender struct {
	primary      service.EmailSender
	fallback     service.EmailSender
	primaryName  string
	fallbackName string
	logger       *log.Logger
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

	// Next: AWS SES
	if hasAnyEnv("AWS_REGION", "SES_FROM_EMAIL", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY") {
		s, err := email.NewSESSender(cfg.Redis.URL, "", "")
		if err != nil {
			logger.Printf("⚠️ SES email sender not initialized: %v", err)
		} else if primary == nil {
			primary = s
			primaryName = "SES"
			logger.Println("✅ Email sender initialized (AWS SES)")
		} else if fallback == nil {
			fallback = s
			fallbackName = "SES"
			logger.Println("✅ Email fallback configured (AWS SES)")
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
	tz string,
	sendAt string,
) {
	if workforceSvc == nil && memberSvc == nil {
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
	}
}

// -----------------------------------------------------------------------------
// No-op email sender (used when DISABLE_EMAIL / DISABLE_OTP)
// -----------------------------------------------------------------------------

type noopEmailSender struct{}

func (noopEmailSender) SendHTML(string, string, string) error {
	return nil
}

// -----------------------------------------------------------------------------
// Router
// -----------------------------------------------------------------------------

func setupRouter(
	cfg *config.Config,
	testimonialHandler *handlers.TestimonialHandler,
	authHandler *handlers.AuthHandler,
	adminHandler *handlers.AdminHandler,
	uploadHandler *handlers.UploadHandler,
	eventHandler *handlers.EventHandler,
	adminNotificationHandler *handlers.AdminNotificationHandler,
	approvalRequestHandler *handlers.ApprovalRequestHandler,
	reelHandler *handlers.ReelHandler,
	analyticsHandler *handlers.AnalyticsHandler,
	formHandler *handlers.FormHandler,
	notificationHandler *handlers.NotificationHandler,
	otpHandler *handlers.OTPHandler,
	workforceHandler *handlers.WorkforceHandler,
	memberHandler *handlers.MemberHandler,
	emailTemplateHandler *handlers.EmailTemplateHandler,
) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.RequestID())
	router.Use(middleware.Logger(cfg.App.LogLevel))
	router.Use(middleware.SecurityHeaders())
	router.Use(middleware.CORS(&cfg.CORS))
	router.Use(middleware.RateLimiter(middleware.RateLimiterOptions{
		RedisURL: cfg.Redis.URL,
		Prefix:   "rl",
	}))

	// Swagger + basic health
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	router.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

	api := router.Group("/api/v1")

	secure := strings.TrimSpace(cfg.App.Environment) == "production"
	authGuard := middleware.AuthMiddleware(cfg.JWT.Secret)
	sessionGuard := middleware.SessionTimeout(30*time.Minute, secure)

	// AUTH
	auth := api.Group("/auth")
	auth.Use(middleware.DeviceFingerprint(secure))
	auth.POST("/login", authHandler.Login)
	// Backwards-compatible aliases for older admin clients
	auth.POST("/login/verify-otp", authHandler.VerifyLoginOTP)
	auth.POST("/login/resend-otp", authHandler.ResendLoginOTP)
	auth.POST("/register", authHandler.Register)
	auth.POST("/password-reset/request", authHandler.RequestPasswordReset)
	auth.POST("/password-reset/confirm", authHandler.ConfirmPasswordReset)
	auth.POST("/otp/verify", authHandler.VerifyLoginOTP)
	auth.POST("/otp/resend", authHandler.ResendLoginOTP)

	authProtected := auth.Group("")
	authProtected.Use(authGuard, sessionGuard)
	authProtected.GET("/me", authHandler.GetCurrentUser)
	authProtected.PATCH("/profile", authHandler.UpdateProfile)
	authProtected.POST("/change-password", authHandler.ChangePassword)
	authProtected.DELETE("/account", authHandler.DeleteAccount)
	authProtected.POST("/clear-data", authHandler.ClearData)
	authProtected.POST("/refresh", authHandler.RefreshToken)
	authProtected.POST("/logout", authHandler.Logout)

	// OTP
	api.POST("/otp/send", otpHandler.SendOTP)
	api.POST("/otp/verify", otpHandler.VerifyOTP)

	// Public testimonials
	api.GET("/testimonials", testimonialHandler.GetPaginatedTestimonials)
	api.GET("/testimonials/all", testimonialHandler.GetAllTestimonials)
	api.GET("/testimonials/:id", testimonialHandler.GetTestimonialByID)
	api.POST("/testimonials", testimonialHandler.CreateTestimonial)

	// Public events/reels
	api.GET("/events", eventHandler.List)
	api.GET("/events/:id", eventHandler.Get)
	api.GET("/reels", reelHandler.List)

	// Notifications (newsletter-style)
	api.POST("/notifications/subscribe", notificationHandler.Subscribe)
	api.POST("/notifications/unsubscribe", notificationHandler.Unsubscribe)
	api.GET("/notifications/unsubscribe", notificationHandler.UnsubscribeByLink)

	// Public forms
	api.GET("/forms/:slug", formHandler.GetPublicForm)
	api.POST("/forms/:slug/submissions", formHandler.SubmitPublicForm)

	// Workforce public apply
	api.POST("/workforce/apply", workforceHandler.Apply)

	// ADMIN
	admin := api.Group("/admin")
	admin.Use(authGuard, sessionGuard, middleware.RoleMiddleware("admin"))

	admin.GET("/dashboard", adminHandler.GetDashboardStats)
	admin.GET("/testimonials/pending", adminHandler.GetPendingTestimonials)

	admin.GET("/users", adminHandler.ListUsers)
	admin.GET("/users/:id", adminHandler.GetUserByID)
	admin.POST("/users", adminHandler.CreateUser)
	admin.PATCH("/users/:id", adminHandler.UpdateUser)
	admin.DELETE("/users/:id", adminHandler.DeleteUser)
	admin.POST("/users/:id/approve", adminHandler.ApproveUser)

	admin.GET("/analytics", analyticsHandler.GetAdminAnalytics)

	// Forms
	// Forms
admin.GET("/forms", formHandler.ListAdminForms)
admin.GET("/forms/:id", formHandler.GetAdminForm)
admin.POST("/forms", formHandler.CreateAdminForm)
admin.PUT("/forms/:id", formHandler.UpdateAdminForm)
admin.DELETE("/forms/:id", formHandler.DeleteAdminForm)
admin.POST("/forms/:id/publish", formHandler.PublishAdminForm)

admin.GET("/forms/:id/submissions", formHandler.ListAdminSubmissions)
admin.GET("/forms/:id/submissions/export.pdf", formHandler.ExportAdminSubmissionsPDF) // ✅ ADD THIS
admin.GET("/forms/:id/submissions/stats", formHandler.GetFormSubmissionStats)

admin.GET("/forms/stats", formHandler.GetFormStats)


	// Notifications (admin)
	admin.GET("/notifications/subscribers", notificationHandler.ListSubscribers)
	admin.POST("/notifications/send", notificationHandler.SendNotification)
	admin.GET("/notifications/inbox", adminNotificationHandler.List)
	admin.PATCH("/notifications/:id/read", adminNotificationHandler.MarkRead)
	admin.POST("/notifications/read-all", adminNotificationHandler.MarkAllRead)

	// Approval requests
	admin.GET("/requests", approvalRequestHandler.List)
	admin.GET("/requests/timeline", approvalRequestHandler.Timeline)

	// Email templates
	admin.POST("/email/templates/send", emailTemplateHandler.SendTemplate)

	// Uploads
	admin.POST("/uploads/images", uploadHandler.UploadImage)

	// Events
	admin.GET("/events", eventHandler.List)
	admin.POST("/events", eventHandler.Create)
	admin.PUT("/events/:id", eventHandler.Update)
	admin.DELETE("/events/:id", eventHandler.Delete)
	admin.POST("/events/:id/image", eventHandler.UploadImage)
	admin.POST("/events/:id/banner", eventHandler.UploadBanner)

	// Reels
	admin.GET("/reels", reelHandler.List)
	admin.POST("/reels", reelHandler.Create)
	admin.DELETE("/reels/:id", reelHandler.Delete)

	// Workforce admin
	admin.GET("/workforce", workforceHandler.List)
	admin.POST("/workforce", workforceHandler.Create)
	admin.PUT("/workforce/:id", workforceHandler.Update)
	admin.GET("/workforce/stats", workforceHandler.Stats)
	admin.GET("/workforce/birthdays/stats", workforceHandler.BirthdayStats)
	admin.GET("/workforce/birthdays/month/:month", workforceHandler.BirthdaysByMonth)
	admin.GET("/workforce/birthdays/today", workforceHandler.BirthdaysToday)
	admin.POST("/workforce/birthdays/send-today", workforceHandler.SendBirthdaysToday)

	// Members admin
	admin.GET("/members", memberHandler.List)
	admin.POST("/members", memberHandler.Create)
	admin.PUT("/members/:id", memberHandler.Update)
	admin.DELETE("/members/:id", memberHandler.Delete)
	admin.GET("/members/birthdays/stats", memberHandler.BirthdayStats)
	admin.GET("/members/birthdays/month/:month", memberHandler.BirthdaysByMonth)
	admin.GET("/members/birthdays/today", memberHandler.BirthdaysToday)
	admin.POST("/members/birthdays/send-today", memberHandler.SendBirthdaysToday)

	// Super-admin
	superAdmin := admin.Group("")
	superAdmin.Use(middleware.RoleMiddleware("super_admin"))
	superAdmin.POST("/workforce/:id/approve", workforceHandler.Approve)
	superAdmin.PATCH("/testimonials/:id/approve", testimonialHandler.ApproveTestimonial)
	superAdmin.DELETE("/testimonials/:id", testimonialHandler.DeleteTestimonial)
	superAdmin.PATCH("/events/:id/approve", eventHandler.Approve)

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
	subscriberRepo := repository.NewSubscriberRepository(db)
	notificationRepo := repository.NewNotificationRepository(db)
	approvalRequestRepo := repository.NewApprovalRequestRepository(db)
	adminNotificationRepo := repository.NewAdminNotificationRepository(db)
	ticketSequenceRepo := repository.NewTicketSequenceRepository(db)
	registrationSequenceRepo := repository.NewRegistrationSequenceRepository(db)
	otpRepo := repository.NewOTPRepository(db)
	workforceRepo := repository.NewWorkforceRepository(db)
	memberRepo := repository.NewMemberRepository(db)
	securityEventRepo := repository.NewSecurityEventRepository(db)
	trustedDeviceRepo := repository.NewTrustedDeviceRepository(db)

	// -------------------------------------------------------------------------
	// Email sender (Brevo / SES)
	// -------------------------------------------------------------------------
	var emailSender service.EmailSender
	if disableEmail {
		emailSender = noopEmailSender{}
		logger.Println("⚠️ Email sender disabled (DISABLE_EMAIL/DISABLE_OTP)")
	} else {
		emailSender = initEmailSender(cfg, logger)
	}

	// -------------------------------------------------------------------------
	// Asset uploader (BunnyCDN-based, matches config.Bunny)
	// -------------------------------------------------------------------------
	var bunnyUploader *service.BunnyUploader
	if cfg.Bunny.Enabled() {
		bunnyUploader = service.NewBunnyUploader(
			cfg.Bunny.StorageZone,
			cfg.Bunny.StorageKey,
			cfg.Bunny.StorageRegion,
			cfg.Bunny.PullZone,
			cfg.Bunny.BasePath,
		)
		logger.Println("✅ Bunny uploader initialized")
	} else {
		logger.Println("ℹ️ Bunny uploads disabled (not configured).")
	}
	var assetUploader service.AssetUploader
	if bunnyUploader != nil {
		assetUploader = bunnyUploader
	}

	// -------------------------------------------------------------------------
	// Branding / email template assets
	// -------------------------------------------------------------------------
	templateAssetBaseURL := strings.TrimRight(cfg.App.EmailTemplateAssetBaseURL, "/")
	if templateAssetBaseURL == "" && cfg.Bunny.Enabled() {
		base := strings.TrimRight(cfg.Bunny.PullZone, "/")
		if strings.TrimSpace(cfg.Bunny.BasePath) != "" {
			base += "/" + strings.Trim(cfg.Bunny.BasePath, "/")
		}
		templateAssetBaseURL = base + "/email-templates"
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
		cfg.App.FrontendURL,
	)

	authService := service.NewAuthService(
		userRepo,
		otpService,
		cfg.JWT.Secret,
		cfg.JWT.Expiration,
		emailSender,
		branding,
		securityService,
		trustedDeviceRepo,
		disableOTP,
		disableLoginOTP,
		approvalService,
		adminNotificationService,
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

	publicBaseURL := strings.TrimRight(strings.TrimSpace(cfg.App.PublicURL), "/")
	if publicBaseURL == "" {
		publicBaseURL = strings.TrimRight(strings.TrimSpace(cfg.App.FrontendURL), "/")
	}
	formService := service.NewFormService(
		formRepo,
		eventRepo,
		registrationSequenceRepo,
		emailSender,
		branding,
		publicBaseURL,
	)

	notificationService := service.NewNotificationService(
		subscriberRepo,
		notificationRepo,
		eventRepo,
		emailSender,
		branding,
	)

	workforceService := service.NewWorkforceService(workforceRepo, emailSender, branding)
	memberService := service.NewMemberService(memberRepo, emailSender, branding)
	emailTemplateService := service.NewEmailTemplateService(emailSender, branding)

	// -------------------------------------------------------------------------
	// Handlers
	// -------------------------------------------------------------------------
	testimonialHandler := handlers.NewTestimonialHandler(testimonialService, userRepo)
	authHandler := handlers.NewAuthHandler(authService)
	adminHandler := handlers.NewAdminHandler(adminService)
	uploadHandler := handlers.NewUploadHandler(assetUploader)
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
	analyticsHandler := handlers.NewAnalyticsHandler(db)
	formHandler := handlers.NewFormHandler(formService)
	notificationHandler := handlers.NewNotificationHandler(notificationService)
	otpHandler := handlers.NewOTPHandler(otpService)
	workforceHandler := handlers.NewWorkforceHandler(workforceService)
	memberHandler := handlers.NewMemberHandler(memberService)
	emailTemplateHandler := handlers.NewEmailTemplateHandler(emailTemplateService)

	// -------------------------------------------------------------------------
	// Background jobs
	// -------------------------------------------------------------------------
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	defer cleanupCancel()

	go startFormCleanup(cleanupCtx, logger, formService, cfg.App.FormCleanupInterval)

	schedulerEnabled := isTrueEnv("BIRTHDAY_SCHEDULER_ENABLED")
	if schedulerEnabled {
		lock := newRedisLock(cfg.Redis.URL)
		tz := strings.TrimSpace(os.Getenv("BIRTHDAY_SCHEDULER_TZ"))
		sendAt := strings.TrimSpace(os.Getenv("BIRTHDAY_SCHEDULER_TIME"))
		go startBirthdayScheduler(cleanupCtx, logger, lock, workforceService, memberService, tz, sendAt)
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
		testimonialHandler,
		authHandler,
		adminHandler,
		uploadHandler,
		eventHandler,
		adminNotificationHandler,
		approvalRequestHandler,
		reelHandler,
		analyticsHandler,
		formHandler,
		notificationHandler,
		otpHandler,
		workforceHandler,
		memberHandler,
		emailTemplateHandler,
	)

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
