// cmd/api/main.go
package main

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/authutil"
	"wisdomHouse-backend/internal/cache"
	"wisdomHouse-backend/internal/config"
	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/handlers"
	appLogger "wisdomHouse-backend/internal/logger"
	"wisdomHouse-backend/internal/metrics"
	"wisdomHouse-backend/internal/realtime"
	"wisdomHouse-backend/internal/repository"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/service/payment"
	"wisdomHouse-backend/internal/telemetry"
	"wisdomHouse-backend/internal/validation"
)

func main() {
	// Bootstrap-only logging before the structured logger exists (config
	// determines its level/format, so it can't be initialized any earlier).
	log.Println("Loading configuration...")
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}

	validation.Init()

	env := strings.ToLower(strings.TrimSpace(cfg.App.Environment))
	if env == "" {
		env = "development"
	}
	cfg.App.Environment = env

	appLogger.Init(cfg.App.LogLevel, cfg.App.Environment)
	logger := appLogger.L()

	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		runMigrateCommand(cfg)
		return
	}

	// Distributed tracing — no-op when OTLP_ENDPOINT is not set.
	telProv, err := telemetry.Init(context.Background(), cfg.Telemetry.OTLPEndpoint, "1.0.0", env)
	if err != nil {
		logger.Warn("OpenTelemetry init failed", "error", err)
	} else if telProv != nil {
		defer func() {
			shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if serr := telProv.Shutdown(shutCtx); serr != nil {
				logger.Warn("telemetry shutdown error", "error", serr)
			}
		}()
		logger.Info("OpenTelemetry tracing initialized")
	}

	disableOTP := isTrueEnv("DISABLE_OTP")
	disableLoginOTP := isTrueEnv("DISABLE_LOGIN_OTP")
	disableEmail := isTrueEnv("DISABLE_EMAIL") || disableOTP
	if disableOTP {
		logger.Warn("DISABLE_OTP=true: OTP verification is disabled for login/password reset")
	}
	if disableLoginOTP {
		logger.Warn("DISABLE_LOGIN_OTP=true: OTP challenges are disabled for login")
	}
	if disableEmail {
		logger.Warn("DISABLE_EMAIL=true: outbound email sending is disabled")
	}

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
		logger.Warn("storage uploader not initialized", "error", err)
	} else if uploader != nil {
		assetUploader = uploader
		logger.Info("storage uploader initialized", "summary", uploader.StorageSummary())
	}

	// -------------------------------------------------------------------------
	// Database
	// -------------------------------------------------------------------------
	db, err := database.NewDatabase(&cfg.Database, cfg.App.Environment)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Warn("error closing database", "error", err)
		}
		logger.Info("database connection closed")
	}()

	if err := verifyDatabaseConnection(db); err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}

	// Metrics server — separate port so /metrics is never publicly reachable.
	if metricsPort := strings.TrimSpace(cfg.Telemetry.MetricsPort); metricsPort != "" {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", metrics.Handler())
		metricsServer := &http.Server{
			Addr:         ":" + metricsPort,
			Handler:      metricsMux,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
		}
		go func() {
			logger.Info("metrics endpoint listening", "port", metricsPort)
			if serr := metricsServer.ListenAndServe(); serr != nil && !errors.Is(serr, http.ErrServerClosed) {
				logger.Warn("metrics server error", "error", serr)
			}
		}()
	}

	if err := database.RunMigrations(db.DB, "migrations"); err != nil {
		logger.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	// Legacy escape hatch — prefer the `migrate` subcommand (see runMigrateCommand)
	// which does this without the rest of server startup.
	if isTrueEnv("RUN_AUTOMIGRATE") {
		logger.Info("RUN_AUTOMIGRATE=true: migrations executed, exiting without starting server")
		return
	}

	// -------------------------------------------------------------------------
	// Repositories
	// -------------------------------------------------------------------------
	testimonialRepo := repository.NewTestimonialRepository(db)
	userRepo := repository.NewUserRepository(db)
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
	refreshTokenRepo := repository.NewRefreshTokenRepository(db)
	givingRepo := repository.NewGivingRepository(db)
	attendanceRepo := repository.NewAttendanceRepository(db)
	cellGroupRepo := repository.NewCellGroupRepository(db)
	prayerRequestRepo := repository.NewPrayerRequestRepository(db)
	ministryRepo := repository.NewMinistryRepository(db)
	auditLogRepo := repository.NewAuditLogRepository(db)

	// -------------------------------------------------------------------------
	// Redis cache (optional)
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
			logger.Warn("redis cache not initialized", "error", err)
		} else {
			defer func() {
				if cerr := redisCache.Close(); cerr != nil {
					logger.Warn("error closing redis cache", "error", cerr)
				}
			}()
			logger.Info("redis cache initialized")
		}
	}

	var userCache *cache.UserCache
	if redisCache != nil {
		userCache = cache.NewUserCache(redisCache, cache.DefaultUserCacheTTL)
	}

	// -------------------------------------------------------------------------
	// RSA key pair for JWT RS256
	// -------------------------------------------------------------------------
	rsaKeyPair, err := authutil.LoadRSAKeyPair(cfg.JWT.PrivateKeyPath, cfg.JWT.PublicKeyPath)
	if err != nil {
		logger.Error("failed to load RSA key pair", "error", err)
		os.Exit(1)
	}
	if rsaKeyPair != nil {
		logger.Info("JWT RS256 key pair loaded")
	} else if env != "production" {
		rsaKeyPair, err = authutil.GenerateEphemeralRSAKeyPair()
		if err != nil {
			logger.Warn("could not generate ephemeral RSA key pair, using HS256", "error", err)
		} else {
			logger.Warn("JWT RS256: using ephemeral dev key pair (configure JWT_PRIVATE_KEY_PATH for production)")
		}
	}

	var tokenBlocklist *authutil.TokenBlocklist
	if redisCache != nil {
		tokenBlocklist = authutil.NewTokenBlocklist(redisCache)
		logger.Info("JWT token blocklist (JTI) initialized")
	}

	var geoDetector *authutil.GeoDetector
	if redisCache != nil {
		geoDetector = authutil.NewGeoDetector(redisCache)
		logger.Info("geo anomaly detector initialized")
	}

	// SSE hub — Redis pub/sub fan-out added when Redis is available.
	sseHub := realtime.New(nil)

	// -------------------------------------------------------------------------
	// Email sender
	// -------------------------------------------------------------------------
	var emailSender service.EmailSender
	if disableEmail {
		emailSender = noopEmailSender{}
		logger.Warn("email sender disabled (DISABLE_EMAIL/DISABLE_OTP)")
	} else {
		emailSender = initEmailSender(cfg)
		emailSender = observedEmailSender{inner: emailSender}
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
		AppTagline:           cfg.App.Tagline,
		Social: email.SocialLinks{
			YouTube:   cfg.App.SocialYouTubeURL,
			Instagram: cfg.App.SocialInstagramURL,
			X:         cfg.App.SocialXURL,
			WhatsApp:  cfg.App.SocialWhatsAppURL,
			Facebook:  cfg.App.SocialFacebookURL,
			TikTok:    cfg.App.SocialTikTokURL,
		},
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

	otpService := service.NewOTPService(otpRepo, emailSender, branding, userRepo, redisCache)

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
		db.DB,
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
		service.AuthServiceOptions{
			PasswordHashCost: cfg.Auth.PasswordHashCost,
			PasswordPolicy:   authutil.PolicyFromConfig(cfg.Auth.PasswordMinLength),
			HIBPEnabled:      cfg.Auth.HIBPEnabled,
			GeoDetector:      geoDetector,
		},
	)

	adminService := service.NewAdminService(
		testimonialRepo,
		userRepo,
		auditLogRepo,
		approvalService,
		adminNotificationService,
		emailSender,
		branding,
	)

	assetService := service.NewAssetService(assetRepo, assetUploader)
	adminEmailService := service.NewAdminEmailService(formRepo, emailTemplateRepo, adminEmailDeliveryRepo, emailSender, branding)
	emailTemplateRegistryService := service.NewEmailTemplateRegistryService(emailTemplateRepo)

	workforceService := service.NewWorkforceService(workforceRepo, adminNotificationService, approvalService, emailSender, branding)
	leadershipService := service.NewLeadershipService(leadershipRepo, formRepo, adminNotificationService, approvalService, emailSender, branding)
	memberService := service.NewMemberService(memberRepo, formRepo, eventRepo, emailSender, branding, cfg.Auth.SecretKey)

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

	// Payment providers — only registered when keys are configured.
	paymentProviders := map[string]payment.Provider{}
	if strings.TrimSpace(cfg.Payment.PaystackSecretKey) != "" {
		paymentProviders["paystack"] = payment.NewPaystack(cfg.Payment.PaystackSecretKey, cfg.Payment.PaystackWebhookSecret)
		logger.Info("Paystack payment provider initialized")
	}
	if strings.TrimSpace(cfg.Payment.StripeSecretKey) != "" {
		paymentProviders["stripe"] = payment.NewStripe(cfg.Payment.StripeSecretKey, cfg.Payment.StripeWebhookSecret)
		logger.Info("Stripe payment provider initialized")
	}
	givingService := service.NewGivingService(givingRepo, paymentProviders)
	attendanceService := service.NewAttendanceService(attendanceRepo)
	cellGroupService := service.NewCellGroupService(cellGroupRepo)
	prayerRequestService := service.NewPrayerRequestService(prayerRequestRepo, cfg.Auth.SecretKey)
	ministryService := service.NewMinistryService(ministryRepo)

	// -------------------------------------------------------------------------
	// Handlers
	// -------------------------------------------------------------------------
	testimonialHandler := handlers.NewTestimonialHandler(testimonialService, userRepo)
	var rsaPrivKey *rsa.PrivateKey
	if rsaKeyPair != nil {
		rsaPrivKey = rsaKeyPair.Private
	}

	authHandler := handlers.NewAuthHandler(authService, handlers.AuthHandlerOptions{
		JWTSecret:                    cfg.JWT.Secret,
		RSAPrivateKey:                rsaPrivKey,
		Secure:                       strings.TrimSpace(cfg.App.Environment) == "production",
		AccessTokenTTL:               cfg.JWT.AccessTokenTTL,
		RefreshTokenTTL:              cfg.JWT.RefreshTokenTTL,
		RememberMeTTL:                cfg.Auth.RememberMeTTL,
		SessionIdleTimeout:           cfg.Auth.SessionIdleTimeout,
		RememberedSessionIdleTimeout: cfg.Auth.RememberedSessionIdleTimeout,
		PostLoginRedirectURL:         cfg.App.AdminPortalURL,
		AuthSecretKey:                cfg.Auth.SecretKey,
		GoogleClientID:               cfg.Auth.GoogleClientID,
		GoogleClientSecret:           cfg.Auth.GoogleClientSecret,
		GoogleRedirectURL:            cfg.Auth.GoogleRedirectURL,
		GoogleHostedDomain:           cfg.Auth.GoogleHostedDomain,
		RefreshTokenRepo:             refreshTokenRepo,
		Blocklist:                    tokenBlocklist,
		PasswordHashCost:             cfg.Auth.PasswordHashCost,
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
		userRepo,
		emailSender,
		emailTemplateRepo,
		branding,
	)
	leadershipHandler := handlers.NewLeadershipHandler(leadershipService, assetUploader, userRepo)
	memberHandler := handlers.NewMemberHandler(memberService)
	emailTemplateHandler := handlers.NewEmailTemplateHandler(emailTemplateService)
	emailTemplateRegistryHandler := handlers.NewEmailTemplateRegistryHandler(emailTemplateRegistryService)
	sermonHandler := handlers.NewSermonHandler(sermonService)
	storeHandler := handlers.NewStoreHandler(storeService)
	givingHandler := handlers.NewGivingHandler()
	givingPaymentsHandler := handlers.NewGivingPaymentsHandler(givingService)
	attendanceHandler := handlers.NewAttendanceHandler(attendanceService)
	cellGroupHandler := handlers.NewCellGroupHandler(cellGroupService)
	prayerRequestHandler := handlers.NewPrayerRequestHandler(prayerRequestService)
	ministryHandler := handlers.NewMinistryHandler(ministryService)
	sseHandler := handlers.NewSSEHandler(sseHub)
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

	go sseHub.Start(cleanupCtx)
	go startFormCleanup(cleanupCtx, formService, cfg.App.FormCleanupInterval)
	go startFormReminderScheduler(cleanupCtx, newRedisLock(cfg.Redis.URL), formService, time.Hour, 24*time.Hour)

	// DB pool stats poller — feeds Prometheus gauges every 15s.
	if sqlDB, serr := db.DB.DB(); serr == nil {
		go func() {
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-cleanupCtx.Done():
					return
				case <-ticker.C:
					metrics.PollDBStats(sqlDB)
				}
			}
		}()
	}

	schedulerEnabled := isTrueEnv("BIRTHDAY_SCHEDULER_ENABLED")
	if schedulerEnabled {
		lock := newRedisLock(cfg.Redis.URL)
		tz := strings.TrimSpace(os.Getenv("BIRTHDAY_SCHEDULER_TZ"))
		sendAt := strings.TrimSpace(os.Getenv("BIRTHDAY_SCHEDULER_TIME"))
		go startBirthdayScheduler(cleanupCtx, lock, workforceService, memberService, leadershipService, tz, sendAt)
	}
	if isTrueEnv("BIRTHDAY_SCHEDULER_ONLY") {
		if !schedulerEnabled {
			logger.Warn("BIRTHDAY_SCHEDULER_ONLY=true but BIRTHDAY_SCHEDULER_ENABLED=false")
		}
		logger.Info("birthday scheduler worker mode enabled (API server disabled)")
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		sig := <-quit
		logger.Info("received signal", "signal", sig.String())
		cleanupCancel()
		return
	}

	// -------------------------------------------------------------------------
	// HTTP server
	// -------------------------------------------------------------------------
	healthHandler := handlers.NewHealthHandler(db, redisCache)

	var rsaPubKey *rsa.PublicKey
	if rsaKeyPair != nil {
		rsaPubKey = rsaKeyPair.Public
	}

	router := setupRouter(
		cfg,
		userRepo,
		userCache,
		rsaPubKey,
		tokenBlocklist,
		healthHandler,
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
		givingPaymentsHandler,
		attendanceHandler,
		cellGroupHandler,
		prayerRequestHandler,
		ministryHandler,
		sseHandler,
		auditLogRepo,
	)

	if err := router.SetTrustedProxies(cfg.Server.TrustedProxies); err != nil {
		logger.Error("invalid SERVER_TRUSTED_PROXIES", "error", err)
		os.Exit(1)
	}
	logger.Info("trusted proxies configured", "proxies", cfg.Server.TrustedProxies)

	server := &http.Server{
		Addr:              ":" + cfg.Server.Port,
		Handler:           router,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		MaxHeaderBytes:    cfg.Server.MaxHeaderBytes,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	shutdownErr := make(chan error, 1)
	go func() {
		host := "localhost"
		if cfg.App.Environment == "production" {
			host = "0.0.0.0"
		}
		logger.Info("server starting", "address", fmt.Sprintf("http://%s:%s", host, cfg.Server.Port))

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
		logger.Info("received signal", "signal", sig.String())
	case err := <-shutdownErr:
		if err != nil {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
		logger.Info("server exited")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger.Info("shutting down server gracefully")
	server.SetKeepAlivesEnabled(false)
	cleanupCancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}
	logger.Info("server exited gracefully")
}
