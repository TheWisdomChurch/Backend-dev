package main

import (
	"fmt"
	"log"

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
// @version 1.0
// @description Backend API for Wisdom House Church Testimonials
// @host localhost:8080
// @BasePath /api/v1
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("❌ Failed to load config: %v", err)
	}

	// Set Gin mode
	gin.SetMode(cfg.Server.GinMode)

	log.Println("🚀 Starting Wisdom House Backend API")
	log.Printf("📡 Port: %s", cfg.Server.Port)
	log.Printf("🗄️  Database: %s:%s/%s", cfg.Database.Host, cfg.Database.Port, cfg.Database.DBName)

	// 1. Connect to Database
	log.Println("🔌 Connecting to database...")
	db, err := database.NewDatabase(&cfg.Database)
	if err != nil {
		log.Fatalf("❌ Failed to connect to database: %v", err)
	}
	defer db.Close()

	// 2. Verify database connection
	log.Println("📊 Verifying database connection...")
	if err := verifyDatabaseConnection(db); err != nil {
		log.Fatalf("❌ Database connection failed: %v", err)
	}
	log.Println("✅ Database connection verified")

	// 3. Initialize repository, service, and handlers
	testimonialRepo := repository.NewTestimonialRepository(db)
	testimonialService := service.NewTestimonialService(testimonialRepo)
	testimonialHandler := handlers.NewTestimonialHandler(testimonialService)

	// 4. Setup Gin router
	router := gin.New()

	// Middleware
	router.Use(gin.Recovery())
	router.Use(middleware.Logger())
	router.Use(middleware.CORS(&cfg.CORS))

	// 5. Routes
	setupRoutes(router, testimonialHandler)

	// Swagger documentation
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// 6. Start server
	log.Printf("✅ Server is ready: http://localhost:%s", cfg.Server.Port)
	log.Printf("📊 Health check: http://localhost:%s/health", cfg.Server.Port)
	log.Printf("🗣️  Testimonials: http://localhost:%s/api/v1/testimonials", cfg.Server.Port)
	log.Printf("📚 Swagger docs: http://localhost:%s/swagger/index.html", cfg.Server.Port)

	if err := router.Run(":" + cfg.Server.Port); err != nil {
		log.Fatalf("❌ Failed to start server: %v", err)
	}
}

// verifyDatabaseConnection checks database connection only
func verifyDatabaseConnection(db *database.Database) error {
	// Simple connection test
	var result int
	if err := db.Raw("SELECT 1").Scan(&result).Error; err != nil {
		return fmt.Errorf("database connection failed: %v", err)
	}
	return nil
}

func setupRoutes(router *gin.Engine, testimonialHandler *handlers.TestimonialHandler) {
	// Health check
	router.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "Wisdom House Backend API",
			"version": "1.0.0",
			"status":  "operational",
		})
	})

	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":    "healthy",
			"service":   "wisdom-house-backend",
			"timestamp": gin.H{},
			"version":   "1.0.0",
		})
	})

	// API v1 routes
	api := router.Group("/api/v1")
	{
		// Testimonials endpoints
		testimonials := api.Group("/testimonials")
		{
			testimonials.POST("", testimonialHandler.CreateTestimonial)
			testimonials.GET("", testimonialHandler.GetAllTestimonials)
			testimonials.GET("paginated", testimonialHandler.GetPaginatedTestimonials)
			testimonials.GET("/:id", testimonialHandler.GetTestimonialByID)
			testimonials.PUT("/:id", testimonialHandler.UpdateTestimonial)
			testimonials.DELETE("/:id", testimonialHandler.DeleteTestimonial)
			testimonials.PATCH("/:id/approve", testimonialHandler.ApproveTestimonial)
		}

		// Simple ping endpoint
		api.GET("/ping", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"message":   "pong",
				"timestamp": gin.H{},
				"status":    "success",
			})
		})

		// Placeholder endpoints
		api.GET("/users", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"message": "Users endpoint - ready for implementation",
				"data":    []any{},
				"count":   0,
				"status":  "implemented",
			})
		})

		api.POST("/auth/register", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"message": "Registration endpoint ready for implementation",
				"status":  "success",
			})
		})
	}
}