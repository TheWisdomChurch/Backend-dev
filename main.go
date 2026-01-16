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
	// Initialize structured logger
	logger := log.New(os.Stdout, "🚀 ", log.Ldate|log.Ltime|log.Lshortfile)

	// Load configuration
	logger.Println("Loading configuration...")
	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("❌ Failed to load configuration: %v", err)
	}

	// Print configuration summary (without sensitive data)
	logger.Println("📋 Configuration Summary:")
	logger.Printf("   Environment: %s", cfg.App.Environment)
	logger.Printf("   Server Port: %s", cfg.Server.Port)
	logger.Printf("   Database: %s", cfg.Database.DSN())
	logger.Printf("   CORS Origins: %v", cfg.CORS.AllowedOrigins)
	logger.Printf("   JWT Configured: %v", cfg.JWT.Secret != "")
	logger.Printf("   Log Level: %s", cfg.App.LogLevel)

	// Set Gin mode based on environment
	gin.SetMode(cfg.Server.GinMode)
	if cfg.App.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// 1. Connect to Database
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

	// 2. Verify database connection
	logger.Println("📊 Verifying database connection...")
	if err := verifyDatabaseConnection(db); err != nil {
		logger.Fatalf("❌ Database connection failed: %v", err)
	}
	logger.Println("✅ Database connection verified")

	// 3. Initialize Redis client (optional)
	// redisClient := redis.NewClient(&cfg.Redis)

	// 4. Initialize repository, service, and handlers
	logger.Println("🔄 Initializing application layers...")
	
	// Initialize repositories
	testimonialRepo := repository.NewTestimonialRepository(db)
	userRepo := repository.NewUserRepository(db)
	adminRepo := repository.NewAdminRepository(db)

	// Initialize services
	testimonialService := service.NewTestimonialService(testimonialRepo)
	authService := service.NewAuthService(userRepo, cfg.JWT.Secret, cfg.JWT.Expiration)
	adminService := service.NewAdminService(adminRepo, testimonialRepo)

	// Initialize handlers
	testimonialHandler := handlers.NewTestimonialHandler(testimonialService)
	authHandler := handlers.NewAuthHandler(authService)
	adminHandler := handlers.NewAdminHandler(adminService)

	// 5. Setup Gin router with middleware
	logger.Println("🚦 Setting up router and middleware...")
	router := setupRouter(cfg, testimonialHandler, authHandler, adminHandler)

	// 6. Create HTTP server with timeouts
	server := &http.Server{
		Addr:           ":" + cfg.Server.Port,
		Handler:        router,
		ReadTimeout:    cfg.Server.ReadTimeout,
		WriteTimeout:   cfg.Server.WriteTimeout,
		MaxHeaderBytes: cfg.Server.MaxHeaderBytes,
		IdleTimeout:    120 * time.Second, // Added for better connection management
	}

	// 7. Graceful shutdown setup
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		// FIXED: Use the actual host address for URLs
		host := "localhost"
		if cfg.App.Environment == "production" {
			host = "0.0.0.0"
		}
		
		logger.Printf("✅ Server starting on http://%s:%s", host, cfg.Server.Port)
		logger.Printf("📊 Health check: http://%s:%s/health", host, cfg.Server.Port)
		logger.Printf("🗣️  Testimonials: http://%s:%s/api/v1/testimonials", host, cfg.Server.Port)
		logger.Printf("🔐 Auth: http://%s:%s/api/v1/auth/login", host, cfg.Server.Port)
		logger.Printf("👨‍💼 Admin: http://%s:%s/api/v1/admin/dashboard", host, cfg.Server.Port)
		
		// Only show Swagger in development
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

	// Wait for interrupt signal
	sig := <-quit
	logger.Printf("🛑 Received signal: %v", sig)

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger.Println("🔄 Shutting down server gracefully...")
	
	// Disable keep-alive connections
	server.SetKeepAlivesEnabled(false)
	
	if err := server.Shutdown(ctx); err != nil {
		logger.Fatalf("❌ Server forced to shutdown: %v", err)
	}

	logger.Println("👋 Server exited gracefully")
}

// verifyDatabaseConnection checks database connection only
func verifyDatabaseConnection(db *database.Database) error {
	var result int
	if err := db.Raw("SELECT 1").Scan(&result).Error; err != nil {
		return fmt.Errorf("database connection failed: %v", err)
	}
	return nil
}

// setupRouter configures all routes and middleware
func setupRouter(
	cfg *config.Config,
	testimonialHandler *handlers.TestimonialHandler,
	authHandler *handlers.AuthHandler,
	adminHandler *handlers.AdminHandler,
) *gin.Engine {
	router := gin.New()

	// Global middleware (order matters)
	router.Use(gin.Recovery())
	router.Use(middleware.Logger(cfg.App.LogLevel))
	router.Use(middleware.CORS(&cfg.CORS))
	router.Use(middleware.SecurityHeaders())
	router.Use(middleware.RequestID())
	router.Use(middleware.RateLimiter()) // Added rate limiting

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

	// API v1 routes
	api := router.Group("/api/v1")
	{
		// ========== PUBLIC ENDPOINTS ==========
		// Testimonials endpoints (public - for website)
		testimonials := api.Group("/testimonials")
		{
			testimonials.GET("", testimonialHandler.GetAllTestimonials)
			testimonials.GET("/paginated", testimonialHandler.GetPaginatedTestimonials)
			testimonials.GET("/:id", testimonialHandler.GetTestimonialByID)
			testimonials.POST("", testimonialHandler.CreateTestimonial) // Public submission
		}

		// Auth endpoints
		auth := api.Group("/auth")
		{
			// Public auth endpoints (no auth required)
			auth.POST("/login", authHandler.Login)
			auth.POST("/register", authHandler.Register)
			auth.POST("/refresh", authHandler.RefreshToken)
			auth.POST("/logout", authHandler.Logout)
			
			// Protected auth endpoints (require auth)
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
		// Admin endpoints (protected - JWT required)
		admin := api.Group("/admin")
		admin.Use(middleware.AuthMiddleware(cfg.JWT.Secret))
		admin.Use(middleware.RoleMiddleware("admin")) // Require admin role
		{
			// Testimonial management (admin only)
			admin.PUT("/testimonials/:id", testimonialHandler.UpdateTestimonial)
			admin.DELETE("/testimonials/:id", testimonialHandler.DeleteTestimonial)
			admin.PATCH("/testimonials/:id/approve", testimonialHandler.ApproveTestimonial)
			
			// Admin dashboard
			admin.GET("/dashboard", adminHandler.GetDashboardStats)
			admin.GET("/testimonials/pending", adminHandler.GetPendingTestimonials)
			
			// User management (admin only)
			admin.GET("/users", adminHandler.GetAllUsers)
			admin.GET("/users/:id", adminHandler.GetUserByID)
			admin.POST("/users", adminHandler.CreateUser)
			admin.PUT("/users/:id", adminHandler.UpdateUser)
			admin.DELETE("/users/:id", adminHandler.DeleteUser)
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

		// Config endpoint (public - non-sensitive info only)
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
					"testimonials": true,
					"authentication": cfg.JWT.Secret != "",
					"admin_panel":   true,
				},
			})
		})
	}

	// Swagger documentation (only in development)
	if cfg.App.Environment == "development" {
		router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}

	// 404 handler
	router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"error":        "Not Found",
			"message":      fmt.Sprintf("Route %s %s not found", c.Request.Method, c.Request.URL.Path),
			"path":         c.Request.URL.Path,
			"method":       c.Request.Method,
			"timestamp":    time.Now().UTC().Unix(),
			"documentation": "/swagger/index.html",
		})
	})

	// Add route debugging in development
	if cfg.App.Environment == "development" {
		setupRouteDebugging(router)
	}

	return router
}

var startTime = time.Now()

// setupRouteDebugging prints all registered routes in development
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

// printRoutes prints all registered routes
func printRoutes(router *gin.Engine) {
	fmt.Println("\n📋 Registered Routes:")
	fmt.Println("===================")
	routes := router.Routes()
	for _, route := range routes {
		fmt.Printf("  %-6s %s\n", route.Method, route.Path)
	}
	fmt.Println("===================\n")
	
	// Also print the specific routes we're looking for
	fmt.Println("🔍 Checking for specific routes:")
	fmt.Println("  PUT    /api/v1/auth/update-profile")
	fmt.Println("  POST   /api/v1/auth/change-password")
	fmt.Println("  DELETE /api/v1/auth/delete-account")
	fmt.Println("  POST   /api/v1/auth/clear-data")
	fmt.Println()
}