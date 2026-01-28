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

func NewDatabase(cfg *config.DatabaseConfig, appEnv string) (*Database, error) {
	dsn := cfg.ConnectionString()

	log.Printf("🔌 Connecting to database at %s:%s...", cfg.Host, cfg.Port)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	// Connection pool settings
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// Test connection
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Println("✅ Database connection established successfully")

	// Ensure extensions needed by your schema/defaults exist.
	// - uuid_generate_v4() => uuid-ossp
	// - gen_random_uuid()  => pgcrypto
	if err := ensureExtensions(db); err != nil {
		return nil, fmt.Errorf("failed to ensure database extensions: %w", err)
	}

	// ✅ Dev convenience, Prod safety
	if appEnv != "production" {
		if err := AutoMigrate(db); err != nil {
			return nil, fmt.Errorf("failed to run automigrate: %w", err)
		}
	} else {
		log.Println("ℹ️ Production mode: AutoMigrate is DISABLED. Run SQL migrations with the migrate service/CI step.")
	}

	return &Database{db}, nil
}

func ensureExtensions(db *gorm.DB) error {
	log.Println("🧩 Ensuring required Postgres extensions...")

	// Important: extensions require sufficient privileges.
	// In managed Postgres you may need to enable them via provider UI, but in Docker you can run these.
	if err := db.Exec(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp";`).Error; err != nil {
		return fmt.Errorf(`create extension "uuid-ossp": %w`, err)
	}

	if err := db.Exec(`CREATE EXTENSION IF NOT EXISTS "pgcrypto";`).Error; err != nil {
		return fmt.Errorf(`create extension "pgcrypto": %w`, err)
	}

	log.Println("✅ Extensions ready")
	return nil
}

func AutoMigrate(db *gorm.DB) error {
	log.Println("🔄 Running database AutoMigrate (non-production)...")

	err := db.AutoMigrate(
		&models.User{},
		&models.Testimonial{},
		&models.Event{},
		&models.Reel{},
		&models.Form{},
		&models.FormField{},
		&models.FormSubmission{},
		&models.Subscriber{},
		&models.Notification{},
		&models.NotificationDelivery{},
		&models.OTP{},
		&models.WorkforceMember{},
		&models.SecurityEvent{},
		&models.TrustedDevice{},
	)
	if err != nil {
		return fmt.Errorf("failed to auto-migrate: %w", err)
	}

	log.Println("✅ AutoMigrate completed successfully")
	return nil
}

func (d *Database) Close() error {
	sqlDB, err := d.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
