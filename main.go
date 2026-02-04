// cmd/api/main.go
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
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

// ... keep your imports ...
// add nothing else needed here (same import list)

func isTrueEnv(key string) bool {
	val := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch val {
	case "1", "true", "t", "yes", "y", "on":
		return true
	default:
		return false
	}
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

func startFormCleanup(ctx context.Context, logger *log.Logger, svc service.FormService, interval time.Duration) {
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

type noopEmailSender struct{}

func (noopEmailSender) SendHTML(string, string, string) error {
	return nil
}

func setupRouter(
	cfg *config.Config,
	testimonialHandler *handlers.TestimonialHandler,
	authHandler *handlers.AuthHandler,
	adminHandler *handlers.AdminHandler,
	uploadHandler *handlers.UploadHandler,
	eventHandler *handlers.EventHandler,
	reelHandler *handlers.ReelHandler,
	analyticsHandler *handlers.AnalyticsHandler,
	formHandler *handlers.FormHandler,
	notificationHandler *handlers.NotificationHandler,
	otpHandler *handlers.OTPHandler,
	workforceHandler *handlers.WorkforceHandler,
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

	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	router.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

	api := router.Group("/api/v1")

	secure := strings.TrimSpace(cfg.App.Environment) == "production"
	authGuard := middleware.AuthMiddleware(cfg.JWT.Secret)
	sessionGuard := middleware.SessionTimeout(30*time.Minute, secure)

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

	api.POST("/otp/send", otpHandler.SendOTP)
	api.POST("/otp/verify", otpHandler.VerifyOTP)

	api.GET("/testimonials", testimonialHandler.GetPaginatedTestimonials)
	api.GET("/testimonials/all", testimonialHandler.GetAllTestimonials)
	api.GET("/testimonials/:id", testimonialHandler.GetTestimonialByID)
	api.POST("/testimonials", testimonialHandler.CreateTestimonial)

	api.GET("/events", eventHandler.List)
	api.GET("/events/:id", eventHandler.Get)

	api.GET("/reels", reelHandler.List)

	api.POST("/notifications/subscribe", notificationHandler.Subscribe)
	api.POST("/notifications/unsubscribe", notificationHandler.Unsubscribe)
	api.GET("/notifications/unsubscribe", notificationHandler.UnsubscribeByLink)

	api.GET("/forms/:slug", formHandler.GetPublicForm)
	api.POST("/forms/:slug/submissions", formHandler.SubmitPublicForm)

	api.POST("/workforce/apply", workforceHandler.Apply)

	admin := api.Group("/admin")
	admin.Use(authGuard, sessionGuard, middleware.RoleMiddleware("admin"))

	admin.GET("/dashboard", adminHandler.GetDashboardStats)
	admin.GET("/testimonials/pending", adminHandler.GetPendingTestimonials)
	admin.PATCH("/testimonials/:id/approve", testimonialHandler.ApproveTestimonial)

	admin.GET("/users", adminHandler.ListUsers)
	admin.GET("/users/:id", adminHandler.GetUserByID)
	admin.POST("/users", adminHandler.CreateUser)
	admin.PATCH("/users/:id", adminHandler.UpdateUser)
	admin.DELETE("/users/:id", adminHandler.DeleteUser)
	admin.POST("/users/:id/approve", adminHandler.ApproveUser)

	admin.GET("/analytics", analyticsHandler.GetAdminAnalytics)

	admin.GET("/forms", formHandler.ListAdminForms)
	admin.GET("/forms/:id", formHandler.GetAdminForm)
	admin.POST("/forms", formHandler.CreateAdminForm)
	admin.PUT("/forms/:id", formHandler.UpdateAdminForm)
	admin.DELETE("/forms/:id", formHandler.DeleteAdminForm)
	admin.POST("/forms/:id/publish", formHandler.PublishAdminForm)
	admin.GET("/forms/:id/submissions", formHandler.ListAdminSubmissions)
	admin.GET("/forms/stats", formHandler.GetFormStats)

	admin.GET("/notifications/subscribers", notificationHandler.ListSubscribers)
	admin.POST("/notifications/send", notificationHandler.SendNotification)

	admin.POST("/email/templates/send", emailTemplateHandler.SendTemplate)

	admin.POST("/uploads/images", uploadHandler.UploadImage)

	admin.GET("/events", eventHandler.List)
	admin.POST("/events", eventHandler.Create)
	admin.PUT("/events/:id", eventHandler.Update)
	admin.DELETE("/events/:id", eventHandler.Delete)
	admin.POST("/events/:id/image", eventHandler.UploadImage)
	admin.POST("/events/:id/banner", eventHandler.UploadBanner)

	admin.GET("/reels", reelHandler.List)
	admin.POST("/reels", reelHandler.Create)
	admin.DELETE("/reels/:id", reelHandler.Delete)

	admin.GET("/workforce", workforceHandler.List)
	admin.POST("/workforce", workforceHandler.Create)
	admin.PUT("/workforce/:id", workforceHandler.Update)
	admin.GET("/workforce/stats", workforceHandler.Stats)

	superAdmin := admin.Group("")
	superAdmin.Use(middleware.RoleMiddleware("super_admin"))
	superAdmin.POST("/workforce/:id/approve", workforceHandler.Approve)

	return router
}

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
	disableEmail := isTrueEnv("DISABLE_EMAIL") || disableOTP
	if disableOTP {
		logger.Println("⚠️ DISABLE_OTP=true: OTP verification is disabled for login/password reset.")
	}
	if disableEmail {
		logger.Println("⚠️ DISABLE_EMAIL=true: outbound email sending is disabled.")
	}

	if cfg.App.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	} else if strings.TrimSpace(cfg.Server.GinMode) != "" {
		gin.SetMode(cfg.Server.GinMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	ensureCORSDefaults(cfg)

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

	if isTrueEnv("RUN_AUTOMIGRATE") {
		logger.Println("✅ RUN_AUTOMIGRATE=true: migrations executed. Exiting without starting server.")
		return
	}

	// Repos
	testimonialRepo := repository.NewTestimonialRepository(db)
	userRepo := repository.NewUserRepository(db)
	adminRepo := repository.NewAdminRepository(db)
	eventRepo := repository.NewEventRepository(db)
	reelRepo := repository.NewReelRepository(db)
	formRepo := repository.NewFormRepository(db)
	subscriberRepo := repository.NewSubscriberRepository(db)
	notificationRepo := repository.NewNotificationRepository(db)
	otpRepo := repository.NewOTPRepository(db)
	workforceRepo := repository.NewWorkforceRepository(db)
	securityEventRepo := repository.NewSecurityEventRepository(db)
	trustedDeviceRepo := repository.NewTrustedDeviceRepository(db)

	// Email sender
	var emailQueue service.EmailSender
	if disableEmail {
		emailQueue = noopEmailSender{}
		logger.Println("⚠️ Email queue disabled (DISABLE_EMAIL/DISABLE_OTP)")
	} else {
		var emailSender email.ContextEmailSender
		var err error

		if cfg.SES.Enabled() {
			emailSender, err = email.NewSESSender(cfg.Redis.URL, cfg.AWS.Region, cfg.SES.FromEmail)
			if err == nil {
				logger.Println("✅ Email sender initialized (AWS SES)")
			}
		} else {
			emailSender, err = email.NewSender(
				cfg.Redis.URL,
				cfg.SMTP.Host,
				cfg.SMTP.Port,
				cfg.SMTP.User,
				cfg.SMTP.Password,
				cfg.SMTP.From,
				cfg.SMTP.TLS,
			)
			if err == nil {
				logger.Println("✅ Email sender initialized (SMTP)")
			}
		}
		if err != nil {
			if cfg.App.Environment == "production" {
				logger.Fatalf("❌ Failed to initialize email sender (required in production): %v", err)
			}
			logger.Printf("⚠️ Email sender not initialized (emails will not send): %v", err)
		}

		if emailSender != nil {
			q := email.NewQueue(emailSender, logger, 2000)
			q.Start(3) // 3 workers is fine for small/medium traffic
			emailQueue = q
			logger.Println("✅ Email queue started")
		} else {
			logger.Println("⚠️ Email queue not started (no sender)")
		}
	}

	// Bunny uploader
	var bunnyUploader *service.BunnyUploader
	if cfg.Bunny.Enabled() {
		bunnyUploader = service.NewBunnyUploader(
			cfg.Bunny.StorageZone,
			cfg.Bunny.StorageKey,
			cfg.Bunny.StorageRegion,
			cfg.Bunny.PullZone,
			cfg.Bunny.BasePath,
		)
	} else {
		logger.Println("ℹ️ Bunny uploads disabled (not configured).")
	}

	// Branding
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

	// ✅ Services (changed: pass emailQueue instead of emailSender)
	testimonialService := service.NewTestimonialService(testimonialRepo, bunnyUploader)

	otpService := service.NewOTPService(otpRepo, emailQueue, branding, userRepo)

	securityService := service.NewSecurityService(
		securityEventRepo,
		trustedDeviceRepo,
		emailQueue,
		branding,
		cfg.App.FrontendURL,
	)

	authService := service.NewAuthService(
		userRepo,
		otpService,
		cfg.JWT.Secret,
		cfg.JWT.Expiration,
		emailQueue,
		branding,
		securityService,
		trustedDeviceRepo,
		disableOTP,
	)

	adminService := service.NewAdminService(adminRepo, testimonialRepo, userRepo)
	formService := service.NewFormService(formRepo, eventRepo)

	notificationService := service.NewNotificationService(
		subscriberRepo,
		notificationRepo,
		eventRepo,
		emailQueue,
		branding,
	)

	workforceService := service.NewWorkforceService(workforceRepo, emailQueue, branding)
	emailTemplateService := service.NewEmailTemplateService(emailQueue, branding)

	// Handlers (unchanged)
	testimonialHandler := handlers.NewTestimonialHandler(testimonialService)
	authHandler := handlers.NewAuthHandler(authService)
	adminHandler := handlers.NewAdminHandler(adminService)
	uploadHandler := handlers.NewUploadHandler(bunnyUploader)
	eventHandler := handlers.NewEventHandler(eventRepo, bunnyUploader)
	reelHandler := handlers.NewReelHandler(reelRepo)
	analyticsHandler := handlers.NewAnalyticsHandler(db)
	formHandler := handlers.NewFormHandler(formService)
	notificationHandler := handlers.NewNotificationHandler(notificationService)
	otpHandler := handlers.NewOTPHandler(otpService)
	workforceHandler := handlers.NewWorkforceHandler(workforceService)
	emailTemplateHandler := handlers.NewEmailTemplateHandler(emailTemplateService)

	// Background jobs
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	defer cleanupCancel()
	go startFormCleanup(cleanupCtx, logger, formService, cfg.App.FormCleanupInterval)

	// Router
	router := setupRouter(
		cfg,
		testimonialHandler,
		authHandler,
		adminHandler,
		uploadHandler,
		eventHandler,
		reelHandler,
		analyticsHandler,
		formHandler,
		notificationHandler,
		otpHandler,
		workforceHandler,
		emailTemplateHandler,
	)

	// Server
	server := &http.Server{
		Addr:              ":" + cfg.Server.Port,
		Handler:           router,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		MaxHeaderBytes:    cfg.Server.MaxHeaderBytes,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Graceful shutdown (unchanged)
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
