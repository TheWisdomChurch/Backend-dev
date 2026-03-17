// internal/database/postgres.go
package database

import (
	"fmt"
	"log"
	"os"
	"strings"
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

// NewDatabase connects using cfg.ConnectionString() (DATABASE_URL preferred).
// appEnv should be "development" or "production" (from cfg.App.Environment).
func NewDatabase(cfg *config.DatabaseConfig, appEnv string) (*Database, error) {
	dsn := cfg.ConnectionString()

	// ✅ Log the actual DSN used (password redacted)
	log.Printf("🔌 Connecting to database: %s", cfg.DSN())

	gormLogger := logger.Default
	if strings.ToLower(appEnv) == "production" {
		gormLogger = gormLogger.LogMode(logger.Warn)
	} else {
		gormLogger = gormLogger.LogMode(logger.Info)
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	// Connection pool settings
	if cfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	} else {
		sqlDB.SetConnMaxLifetime(time.Hour)
	}

	// Test connection
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}
	log.Println("✅ Database connection established successfully")

	// Extensions (best-effort in production)
	if err := ensureExtensions(db, appEnv); err != nil {
		return nil, fmt.Errorf("failed to ensure database extensions: %w", err)
	}

	// ✅ Seamless behavior:
	// - In dev: AutoMigrate runs automatically
	// - In prod: AutoMigrate runs ONLY if RUN_AUTOMIGRATE=true (migrate job)
	runAuto := strings.ToLower(appEnv) != "production" || strings.ToLower(os.Getenv("RUN_AUTOMIGRATE")) == "true"
	if runAuto {
		if err := AutoMigrate(db); err != nil {
			return nil, fmt.Errorf("failed to run automigrate: %w", err)
		}
	} else {
		log.Println("ℹ️ Production mode: AutoMigrate is disabled. Run migrate job with RUN_AUTOMIGRATE=true.")
	}

	return &Database{db}, nil
}

func ensureExtensions(db *gorm.DB, appEnv string) error {
	log.Println("🧩 Ensuring required Postgres extensions...")

	// In managed Postgres (Supabase), extension creation may require enabling in dashboard.
	// So in production we warn instead of hard-failing.
	tryExec := func(sql string, name string) error {
		if err := db.Exec(sql).Error; err != nil {
			if strings.ToLower(appEnv) == "production" {
				log.Printf("⚠️ Could not ensure extension %s (enable it in Supabase if required): %v", name, err)
				return nil
			}
			return fmt.Errorf("ensure extension %s: %w", name, err)
		}
		return nil
	}

	if err := tryExec(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp";`, "uuid-ossp"); err != nil {
		return err
	}
	if err := tryExec(`CREATE EXTENSION IF NOT EXISTS "pgcrypto";`, "pgcrypto"); err != nil {
		return err
	}

	log.Println("✅ Extensions check done")
	return nil
}

func AutoMigrate(db *gorm.DB) error {
	log.Println("🔄 Running database AutoMigrate...")

	err := db.AutoMigrate(
		&models.User{},
		&models.AdminNotification{},
		&models.ApprovalRequest{},
		&models.Testimonial{},
		&models.Event{},
		&models.Reel{},
		&models.Form{},
		&models.FormField{},
		&models.FormSubmission{},
		&models.FormCalendarReminder{},
		&models.FormCampaignDelivery{},
		&models.AdminEmailDelivery{},
		&models.RegistrationSequence{},
		&models.Asset{},
		&models.EmailTemplate{},
		&models.Subscriber{},
		&models.Notification{},
		&models.NotificationDelivery{},
		&models.OTP{},
		&models.TicketSequence{},
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
