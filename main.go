// cmd/api/main.go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"wisdomHouse-backend/internal/config"
	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/handlers"
	"wisdomHouse-backend/internal/middleware"
	"wisdomHouse-backend/internal/repository"
	"wisdomHouse-backend/internal/service"
)

// @title Wisdom House Backend API
// @version 1.0.0
// @description Backend API for Wisdom House Church Testimonials
// @contact.name API Support
// @contact.url http://wisdomhousechurch.com/support
// @contact.email support@wisdomhousechurch.com
// @license.name MIT
// @license.url https://opensource.org/licenses/MIT
// @host localhost:8080
// @BasePath /api/v1
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name X-API-Key
func main() {
	logger := log.New(os.Stdout, "🚀 ", log.Ldate|log.Ltime|log.Lshortfile)

	logger.Println("Loading configuration...")
	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("❌ Failed to load configuration: %v", err)
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
	db, err := database.NewDatabase(&cfg.Database)
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

	// Existing repositories
	testimonialRepo := repository.NewTestimonialRepository(db)
	userRepo := repository.NewUserRepository(db)
	adminRepo := repository.NewAdminRepository(db)

	// NEW repositories
	eventRepo := repository.NewEventRepository(db)
	reelRepo := repository.NewReelRepository(db)

	// Existing services
	testimonialService := service.NewTestimonialService(testimonialRepo)
	authService := service.NewAuthService(userRepo, cfg.JWT.Secret, cfg.JWT.Expiration)
	adminService := service.NewAdminService(adminRepo, testimonialRepo)

	// Existing handlers
	testimonialHandler := handlers.NewTestimonialHandler(testimonialService)
	authHandler := handlers.NewAuthHandler(authService)
	adminHandler := handlers.NewAdminHandler(adminService)

	// NEW handlers
	eventHandler := handlers.NewEventHandler(eventRepo)
	reelHandler := handlers.NewReelHandler(reelRepo)
	analyticsHandler := handlers.NewAnalyticsHandler(db) // uses db for aggregate queries

	// 4) Router
	logger.Println("🚦 Setting up router and middleware...")
	router := setupRouter(cfg,
		testimonialHandler,
		authHandler,
		adminHandler,
		eventHandler,
		reelHandler,
		analyticsHandler,
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

// UPDATED: accept new handlers
func setupRouter(
	cfg *config.Config,
	testimonialHandler *handlers.TestimonialHandler,
	authHandler *handlers.AuthHandler,
	adminHandler *handlers.AdminHandler,
	eventHandler *handlers.EventHandler,
	reelHandler *handlers.ReelHandler,
	analyticsHandler *handlers.AnalyticsHandler,
) *gin.Engine {
	router := gin.New()

	// Global middleware
	router.Use(gin.Recovery())
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
				"health":       "/health",
				"api_docs":     "/swagger/index.html",
				"api_v1":       "/api/v1",
				"testimonials": "/api/v1/testimonials",
				"auth":         "/api/v1/auth",
				"admin":        "/api/v1/admin",
				"events":       "/api/v1/events",
				"reels":        "/api/v1/reels",
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
		// ========== PUBLIC ENDPOINTS ==========
		testimonials := api.Group("/testimonials")
		{
			testimonials.GET("", testimonialHandler.GetAllTestimonials)
			testimonials.GET("/paginated", testimonialHandler.GetPaginatedTestimonials)
			testimonials.GET("/:id", testimonialHandler.GetTestimonialByID)
			testimonials.POST("", testimonialHandler.CreateTestimonial)
		}

		// Auth endpoints
		auth := api.Group("/auth")
		{
			auth.POST("/login", authHandler.Login)
			auth.POST("/register", authHandler.Register)
			auth.POST("/refresh", authHandler.RefreshToken)
			auth.POST("/logout", authHandler.Logout)

			protected := auth.Group("")
			protected.Use(middleware.AuthMiddleware(cfg.JWT.Secret))
			{
				protected.GET("/me", authHandler.GetCurrentUser)
				protected.PUT("/update-profile", authHandler.UpdateProfile)
				protected.POST("/change-password", authHandler.ChangePassword)
				protected.DELETE("/delete-account", authHandler.DeleteAccount)
				protected.POST("/clear-data", authHandler.ClearData)
			}
		}

		// ========== PROTECTED ENDPOINTS ==========

		// Admin endpoints
		admin := api.Group("/admin")
		admin.Use(middleware.AuthMiddleware(cfg.JWT.Secret))
		admin.Use(middleware.RoleMiddleware("admin"))
		{
			// Testimonial management
			admin.PUT("/testimonials/:id", testimonialHandler.UpdateTestimonial)
			admin.DELETE("/testimonials/:id", testimonialHandler.DeleteTestimonial)
			admin.PATCH("/testimonials/:id/approve", testimonialHandler.ApproveTestimonial)

			// Admin dashboard
			admin.GET("/dashboard", adminHandler.GetDashboardStats)
			admin.GET("/testimonials/pending", adminHandler.GetPendingTestimonials)

			// User management
			admin.GET("/users", adminHandler.GetAllUsers)
			admin.GET("/users/:id", adminHandler.GetUserByID)
			admin.POST("/users", adminHandler.CreateUser)
			admin.PUT("/users/:id", adminHandler.UpdateUser)
			admin.DELETE("/users/:id", adminHandler.DeleteUser)

			// NEW: analytics
			admin.GET("/analytics", analyticsHandler.GetAdminAnalytics)
		}

		// NEW: Events endpoints (admin-only)
		events := api.Group("/events")
		events.Use(middleware.AuthMiddleware(cfg.JWT.Secret))
		events.Use(middleware.RoleMiddleware("admin"))
		{
			events.GET("", eventHandler.List)
			events.POST("", eventHandler.Create)
			events.GET("/:id", eventHandler.Get)
			events.PUT("/:id", eventHandler.Update)
			events.DELETE("/:id", eventHandler.Delete)
		}

		// NEW: Reels endpoints (admin-only)
		reels := api.Group("/reels")
		reels.Use(middleware.AuthMiddleware(cfg.JWT.Secret))
		reels.Use(middleware.RoleMiddleware("admin"))
		{
			reels.GET("", reelHandler.List)
			reels.POST("", reelHandler.Create)
			reels.DELETE("/:id", reelHandler.Delete)
		}

		// System endpoints (public)
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
