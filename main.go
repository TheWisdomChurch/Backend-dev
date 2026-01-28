// cmd/api/main.go
package main

import (
	"context"
	"fmt"
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
)

// @title Wisdom House Backend API
// @version 1.0.0
// @description Backend API for Wisdom House Church
// @host localhost:8080
// @BasePath /api/v1
func main() {
	logger := log.New(os.Stdout, "🚀 ", log.Ldate|log.Ltime|log.Lshortfile)

	logger.Println("Loading configuration...")
	cfg, err := config.Load()
	if err != nil {
		// ✅ DEPLOY-TODAY WORKAROUND:
		// If config validation currently forces BunnyCDN, allow boot without Bunny.
		// We temporarily set placeholder Bunny env vars to satisfy validation,
		// reload config, then disable Bunny in-memory.
		if strings.Contains(err.Error(), "BunnyCDN config incomplete") {
			logger.Printf("⚠️ BunnyCDN config incomplete; continuing with Bunny uploads DISABLED: %v", err)

			// Placeholder values (only to satisfy validation if it checks non-empty).
			// These are immediately disabled after load, so uploader won't be used.
			_ = os.Setenv("BUNNYCDN_STORAGE_ZONE", "DISABLED")
			_ = os.Setenv("BUNNYCDN_STORAGE_KEY", "DISABLED")
			_ = os.Setenv("BUNNYCDN_STORAGE_REGION", "DISABLED")
			_ = os.Setenv("BUNNYCDN_PULL_ZONE", "DISABLED")
			if os.Getenv("BUNNYCDN_BASE_PATH") == "" {
				_ = os.Setenv("BUNNYCDN_BASE_PATH", "")
			}

			cfg, err = config.Load()
			if err != nil {
				logger.Fatalf("❌ Failed to load configuration even after Bunny bypass: %v", err)
			}

			// Disable Bunny in-memory so your app behaves as "Bunny not configured".
			cfg.Bunny.StorageZone = ""
			cfg.Bunny.StorageKey = ""
			cfg.Bunny.StorageRegion = ""
			cfg.Bunny.PullZone = ""
			cfg.Bunny.BasePath = ""
		} else {
			logger.Fatalf("❌ Failed to load configuration: %v", err)
		}
	}

	logger.Println("📋 Configuration Summary:")
	logger.Printf("   Environment: %s", cfg.App.Environment)
	logger.Printf("   Server Port: %s", cfg.Server.Port)
	logger.Printf("   Database: %s", cfg.Database.DSN())
	logger.Printf("   CORS Origins: %v", cfg.CORS.AllowedOrigins)
	logger.Printf("   JWT Configured: %v", cfg.JWT.Secret != "")
	logger.Printf("   Log Level: %s", cfg.App.LogLevel)

	gin.SetMode(cfg.Server.GinMode)
	if cfg.App.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// 1) DB
	logger.Println("🔌 Connecting to database...")
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

	// 2) Verify DB
	logger.Println("📊 Verifying database connection...")
	if err := verifyDatabaseConnection(db); err != nil {
		logger.Fatalf("❌ Database connection failed: %v", err)
	}
	logger.Println("✅ Database connection verified")

	// 3) Init repos/services/handlers
	logger.Println("🔄 Initializing application layers...")

	// Repositories
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
	emailSender, err := email.NewSender(
		cfg.Redis.URL,
		cfg.SMTP.Host,
		cfg.SMTP.Port,
		cfg.SMTP.User,
		cfg.SMTP.Password,
		cfg.SMTP.From,
		cfg.SMTP.TLS,
	)
	if err != nil {
		if cfg.App.Environment == "production" {
			logger.Fatalf("❌ Failed to initialize email sender (required in production): %v", err)
		}
		logger.Printf("⚠️ Email sender not initialized (emails will not send): %v", err)
	}

	// Bunny uploader service (optional)
	var bunnyUploader *service.BunnyUploader
	if cfg.Bunny.StorageZone != "" && cfg.Bunny.StorageKey != "" && cfg.Bunny.PullZone != "" && cfg.Bunny.StorageRegion != "" {
		bunnyUploader = service.NewBunnyUploader(
			cfg.Bunny.StorageZone,
			cfg.Bunny.StorageKey,
			cfg.Bunny.StorageRegion,
			cfg.Bunny.PullZone,
			cfg.Bunny.BasePath,
		)
		logger.Printf("📦 Bunny uploader configured: zone=%s region=%s pull=%s base=%s",
			cfg.Bunny.StorageZone,
			cfg.Bunny.StorageRegion,
			cfg.Bunny.PullZone,
			cfg.Bunny.BasePath,
		)
	} else {
		logger.Printf("⚠️ Bunny uploader not configured (uploads disabled)")
	}

	// Services
	branding := email.Branding{
		AppName:        cfg.App.Name,
		LogoURL:        cfg.App.LogoURL,
		PublicURL:      cfg.App.PublicURL,
		FrontendURL:    cfg.App.FrontendURL,
		SupportEmail:   cfg.App.SupportEmail,
		PastorName:     cfg.App.PastorName,
		AdminPortalURL: cfg.App.AdminPortalURL,
	}

	testimonialService := service.NewTestimonialService(testimonialRepo, bunnyUploader)
	otpService := service.NewOTPService(otpRepo, emailSender, branding)
	securityService := service.NewSecurityService(securityEventRepo, trustedDeviceRepo, emailSender, branding, cfg.App.FrontendURL)
	authService := service.NewAuthService(userRepo, otpService, cfg.JWT.Secret, cfg.JWT.Expiration, emailSender, branding, securityService, trustedDeviceRepo)
	adminService := service.NewAdminService(adminRepo, testimonialRepo, userRepo)

	// Handlers
	testimonialHandler := handlers.NewTestimonialHandler(testimonialService)
	authHandler := handlers.NewAuthHandler(authService)
	adminHandler := handlers.NewAdminHandler(adminService)

	// pass bunnyUploader into NewEventHandler
	eventHandler := handlers.NewEventHandler(eventRepo, bunnyUploader)

	reelHandler := handlers.NewReelHandler(reelRepo)
	analyticsHandler := handlers.NewAnalyticsHandler(db)

	formService := service.NewFormService(formRepo, eventRepo)
	formHandler := handlers.NewFormHandler(formService)

	notificationService := service.NewNotificationService(subscriberRepo, notificationRepo, eventRepo, emailSender, branding)
	notificationHandler := handlers.NewNotificationHandler(notificationService)

	otpHandler := handlers.NewOTPHandler(otpService)

	workforceService := service.NewWorkforceService(workforceRepo, emailSender, branding)
	workforceHandler := handlers.NewWorkforceHandler(workforceService)

	// 4) Router
	logger.Println("🚦 Setting up router and middleware...")
	router := setupRouter(cfg,
		testimonialHandler,
		authHandler,
		adminHandler,
		eventHandler,
		reelHandler,
		analyticsHandler,
		formHandler,
		notificationHandler,
		otpHandler,
		workforceHandler,
	)

	// 5) Server
	server := &http.Server{
		Addr:           ":" + cfg.Server.Port,
		Handler:        router,
		ReadTimeout:    cfg.Server.ReadTimeout,
		WriteTimeout:   cfg.Server.WriteTimeout,
		MaxHeaderBytes: cfg.Server.MaxHeaderBytes,
		IdleTimeout:    120 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		host := "localhost"
		if cfg.App.Environment == "production" {
			host = "0.0.0.0"
		}

		logger.Printf("✅ Server starting on http://%s:%s", host, cfg.Server.Port)
		logger.Printf("📊 Health check: http://%s:%s/health", host, cfg.Server.Port)
		logger.Printf("🗣️  Testimonials: http://%s:%s/api/v1/testimonials", host, cfg.Server.Port)
		logger.Printf("🔐 Auth: http://%s:%s/api/v1/auth/login", host, cfg.Server.Port)
		logger.Printf("👨‍💼 Admin: http://%s:%s/api/v1/admin/dashboard", host, cfg.Server.Port)
		logger.Printf("📅 Events: http://%s:%s/api/v1/events", host, cfg.Server.Port)
		logger.Printf("🎬 Reels: http://%s:%s/api/v1/reels", host, cfg.Server.Port)
		logger.Printf("📈 Analytics: http://%s:%s/api/v1/admin/analytics", host, cfg.Server.Port)
		logger.Printf("🧾 Forms (admin): http://%s:%s/api/v1/admin/forms", host, cfg.Server.Port)
		logger.Printf("🧾 Forms (public): http://%s:%s/api/v1/forms/:slug", host, cfg.Server.Port)
		logger.Printf("📬 Subscribers: http://%s:%s/api/v1/subscribers", host, cfg.Server.Port)
		logger.Printf("📬 Notifications: http://%s:%s/api/v1/admin/notifications", host, cfg.Server.Port)
		logger.Printf("🔐 OTP: http://%s:%s/api/v1/otp", host, cfg.Server.Port)
		logger.Printf("👥 Workforce apply: http://%s:%s/api/v1/workforce/apply", host, cfg.Server.Port)

		logger.Printf("🖼️  Event image upload: http://%s:%s/api/v1/events/:id/image", host, cfg.Server.Port)
		logger.Printf("🖼️  Event banner upload: http://%s:%s/api/v1/events/:id/banner", host, cfg.Server.Port)

		if cfg.App.Environment == "development" {
			logger.Printf("📚 Swagger docs: http://%s:%s/swagger/index.html", host, cfg.Server.Port)
		}

		logger.Printf("⚙️  Environment: %s", cfg.App.Environment)
		logger.Printf("📈 Auto-migration: Enabled")
		logger.Printf("🗄️  Database: %s", cfg.Database.DBName)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("❌ Failed to start server: %v", err)
		}
	}()

	sig := <-quit
	logger.Printf("🛑 Received signal: %v", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger.Println("🔄 Shutting down server gracefully...")
	server.SetKeepAlivesEnabled(false)

	if err := server.Shutdown(ctx); err != nil {
		logger.Fatalf("❌ Server forced to shutdown: %v", err)
	}

	logger.Println("👋 Server exited gracefully")
}

func verifyDatabaseConnection(db *database.Database) error {
	var result int
	if err := db.Raw("SELECT 1").Scan(&result).Error; err != nil {
		return fmt.Errorf("database connection failed: %v", err)
	}
	return nil
}

// setupRouter wires routes + middleware
func setupRouter(
	cfg *config.Config,
	testimonialHandler *handlers.TestimonialHandler,
	authHandler *handlers.AuthHandler,
	adminHandler *handlers.AdminHandler,
	eventHandler *handlers.EventHandler,
	reelHandler *handlers.ReelHandler,
	analyticsHandler *handlers.AnalyticsHandler,
	formHandler *handlers.FormHandler,
	notificationHandler *handlers.NotificationHandler,
	otpHandler *handlers.OTPHandler,
	workforceHandler *handlers.WorkforceHandler,
) *gin.Engine {
	router := gin.New()
	sessionTimeout := middleware.SessionTimeout(30*time.Minute, cfg.App.Environment == "production")

	// Global middleware
	router.Use(gin.Recovery())
	router.Use(middleware.DeviceFingerprint(cfg.App.Environment == "production"))
	router.Use(middleware.Logger(cfg.App.LogLevel))
	router.Use(middleware.CORS(&cfg.CORS))
	router.Use(middleware.SecurityHeaders())
	router.Use(middleware.RequestID())
	router.Use(middleware.RateLimiter())

	// Basic routes
	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"name":        cfg.App.Name,
			"version":     cfg.App.Version,
			"environment": cfg.App.Environment,
			"status":      "operational",
			"timestamp":   time.Now().UTC(),
			"endpoints": gin.H{
				"health":          "/health",
				"api_docs":        "/swagger/index.html",
				"api_v1":          "/api/v1",
				"testimonials":    "/api/v1/testimonials",
				"auth":            "/api/v1/auth",
				"admin":           "/api/v1/admin",
				"events":          "/api/v1/events",
				"reels":           "/api/v1/reels",
				"forms_admin":     "/api/v1/admin/forms",
				"forms_public":    "/api/v1/forms/:slug",
				"subscribers":     "/api/v1/subscribers",
				"notifications":   "/api/v1/admin/notifications",
				"otp_send":        "/api/v1/otp/send",
				"otp_verify":      "/api/v1/otp/verify",
				"workforce_apply": "/api/v1/workforce/apply",
			},
		})
	})

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"service":   cfg.App.Name,
			"version":   cfg.App.Version,
			"timestamp": time.Now().UTC().Unix(),
			"uptime":    time.Since(startTime).String(),
			"database":  "connected",
		})
	})

	api := router.Group("/api/v1")
	{
		// PUBLIC
		testimonials := api.Group("/testimonials")
		{
			testimonials.GET("", testimonialHandler.GetAllTestimonials)
			testimonials.GET("/paginated", testimonialHandler.GetPaginatedTestimonials)
			testimonials.GET("/:id", testimonialHandler.GetTestimonialByID)
			testimonials.POST("", testimonialHandler.CreateTestimonial)
		}

		publicForms := api.Group("/forms")
		{
			publicForms.GET("/:slug", formHandler.GetPublicForm)
			publicForms.POST("/:slug/submissions", formHandler.SubmitPublicForm)
		}

		subscribers := api.Group("/subscribers")
		{
			subscribers.POST("", notificationHandler.Subscribe)
			subscribers.POST("/unsubscribe", notificationHandler.Unsubscribe)
			subscribers.GET("/unsubscribe", notificationHandler.UnsubscribeByLink)
		}

		api.POST("/workforce/apply", workforceHandler.Apply)

		otp := api.Group("/otp")
		{
			otp.POST("/send", otpHandler.SendOTP)
			otp.POST("/verify", otpHandler.VerifyOTP)
		}

		// AUTH
		auth := api.Group("/auth")
		{
			auth.POST("/login", authHandler.Login)
			auth.POST("/login/verify-otp", authHandler.VerifyLoginOTP)
			auth.POST("/register", authHandler.Register)
			auth.POST("/refresh", authHandler.RefreshToken)
			auth.POST("/logout", authHandler.Logout)
			auth.POST("/password-reset/request", authHandler.RequestPasswordReset)
			auth.POST("/password-reset/confirm", authHandler.ConfirmPasswordReset)

			protected := auth.Group("")
			protected.Use(middleware.AuthMiddleware(cfg.JWT.Secret), sessionTimeout)
			{
				protected.GET("/me", authHandler.GetCurrentUser)
				protected.PUT("/update-profile", authHandler.UpdateProfile)
				protected.POST("/change-password", authHandler.ChangePassword)
				protected.DELETE("/delete-account", authHandler.DeleteAccount)
				protected.POST("/clear-data", authHandler.ClearData)
			}
		}

		// ADMIN (protected)
		admin := api.Group("/admin")
		admin.Use(middleware.AuthMiddleware(cfg.JWT.Secret), sessionTimeout)
		admin.Use(middleware.RoleMiddleware("admin"))
		{
			admin.PUT("/testimonials/:id", testimonialHandler.UpdateTestimonial)
			admin.DELETE("/testimonials/:id", testimonialHandler.DeleteTestimonial)
			admin.PATCH("/testimonials/:id/approve", testimonialHandler.ApproveTestimonial)

			admin.GET("/dashboard", adminHandler.GetDashboardStats)
			admin.GET("/testimonials/pending", adminHandler.GetPendingTestimonials)

			admin.GET("/users", adminHandler.GetAllUsers)
			admin.GET("/users/:id", adminHandler.GetUserByID)
			admin.POST("/users", adminHandler.CreateUser)
			admin.PUT("/users/:id", adminHandler.UpdateUser)
			admin.DELETE("/users/:id", adminHandler.DeleteUser)

			admin.GET("/analytics", analyticsHandler.GetAdminAnalytics)

			admin.GET("/forms", formHandler.ListAdminForms)
			admin.GET("/forms/stats", formHandler.GetFormStats)
			admin.POST("/forms", formHandler.CreateAdminForm)
			admin.GET("/forms/:id", formHandler.GetAdminForm)
			admin.PUT("/forms/:id", formHandler.UpdateAdminForm)
			admin.DELETE("/forms/:id", formHandler.DeleteAdminForm)
			admin.POST("/forms/:id/publish", formHandler.PublishAdminForm)
			admin.GET("/forms/:id/submissions", formHandler.ListAdminSubmissions)

			admin.GET("/subscribers", notificationHandler.ListSubscribers)
			admin.POST("/notifications", notificationHandler.SendNotification)

			admin.GET("/workforce", workforceHandler.List)
			admin.POST("/workforce", workforceHandler.Create)
			admin.PATCH("/workforce/:id", workforceHandler.Update)
			admin.GET("/workforce/stats", workforceHandler.Stats)

			superAdmin := admin.Group("")
			superAdmin.Use(middleware.RoleMiddleware("super_admin"))
			{
				superAdmin.PATCH("/users/:id/approve", adminHandler.ApproveUser)
				superAdmin.PATCH("/workforce/:id/approve", workforceHandler.Approve)
			}
		}

		// EVENTS (admin-only)
		events := api.Group("/events")
		events.Use(middleware.AuthMiddleware(cfg.JWT.Secret), sessionTimeout)
		events.Use(middleware.RoleMiddleware("admin"))
		{
			events.GET("", eventHandler.List)
			events.POST("", eventHandler.Create)
			events.GET("/:id", eventHandler.Get)
			events.PUT("/:id", eventHandler.Update)
			events.DELETE("/:id", eventHandler.Delete)

			events.POST("/:id/image", eventHandler.UploadImage)
			events.POST("/:id/banner", eventHandler.UploadBanner)
		}

		// REELS (admin-only)
		reels := api.Group("/reels")
		reels.Use(middleware.AuthMiddleware(cfg.JWT.Secret), sessionTimeout)
		reels.Use(middleware.RoleMiddleware("admin"))
		{
			reels.GET("", reelHandler.List)
			reels.POST("", reelHandler.Create)
			reels.DELETE("/:id", reelHandler.Delete)
		}

		api.GET("/ping", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"message":   "pong",
				"timestamp": time.Now().UTC().Unix(),
				"status":    "success",
				"service":   cfg.App.Name,
			})
		})

		api.GET("/config", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"app": gin.H{
					"name":        cfg.App.Name,
					"version":     cfg.App.Version,
					"environment": cfg.App.Environment,
				},
				"api": gin.H{
					"base_path": "/api/v1",
					"version":   "1.0.0",
				},
				"features": gin.H{
					"testimonials":    true,
					"authentication":  cfg.JWT.Secret != "",
					"admin_panel":     true,
					"events":          true,
					"reels":           true,
					"admin_analytics": true,
					"forms":           true,
					"notifications":   true,
					"otp":             true,
				},
			})
		})
	}

	if cfg.App.Environment == "development" {
		router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}

	router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"error":         "Not Found",
			"message":       fmt.Sprintf("Route %s %s not found", c.Request.Method, c.Request.URL.Path),
			"path":          c.Request.URL.Path,
			"method":        c.Request.Method,
			"timestamp":     time.Now().UTC().Unix(),
			"documentation": "/swagger/index.html",
		})
	})

	if cfg.App.Environment == "development" {
		setupRouteDebugging(router)
	}

	return router
}

var startTime = time.Now()

func setupRouteDebugging(router *gin.Engine) {
	router.Use(func(c *gin.Context) {
		if !routesPrinted {
			routesPrinted = true
			printRoutes(router)
		}
		c.Next()
	})
}

var routesPrinted bool

func printRoutes(router *gin.Engine) {
	fmt.Println("\n📋 Registered Routes:")
	fmt.Println("===================")
	for _, route := range router.Routes() {
		fmt.Printf("  %-6s %s\n", route.Method, route.Path)
	}
	fmt.Println("===================\n")
}
