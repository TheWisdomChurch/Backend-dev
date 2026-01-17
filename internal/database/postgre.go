package database

import (
	"fmt"
	"log"
	"time"

	"wisdomHouse-backend/internal/config"
	"wisdomHouse-backend/internal/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Database struct {
	*gorm.DB
}

func NewDatabase(cfg *config.DatabaseConfig) (*Database, error) {
	dsn := cfg.ConnectionString()

	log.Printf("🔌 Connecting to database at %s:%s...", cfg.Host, cfg.Port)

	// ✅ PERFECT for auto-migration
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info), // Good for seeing migration SQL
	})

	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	// ✅ Good connection pool settings
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// ✅ Test connection
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Println("✅ Database connection established successfully")

	// ✅ PERFECT: AutoMigrate will create ALL tables automatically
	if err := AutoMigrate(db); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return &Database{db}, nil
}

// ✅ PERFECT: AutoMigrate creates all tables automatically
func AutoMigrate(db *gorm.DB) error {
	log.Println("🔄 Running database migrations...")
	
	// ✅ List all models - GORM will create tables if they don't exist
	err := db.AutoMigrate(
		&models.User{},           // Creates "users" table
		&models.Testimonial{},    // Creates "testimonials" table
		// Add other models here as you create them
	)
	
	if err != nil {
		return fmt.Errorf("failed to auto-migrate: %w", err)
	}
	
	log.Println("✅ Database migrations completed successfully")
	return nil
}

// ✅ Good: Separate Migrate method for manual use
func (d *Database) Migrate() error {
	return AutoMigrate(d.DB)
}

func (d *Database) Close() error {
	sqlDB, err := d.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}