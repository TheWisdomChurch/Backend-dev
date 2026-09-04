package database

import (
	"fmt"
	"os"
	"strings"
	"time"

	"wisdomHouse-backend/internal/config"
	applog "wisdomHouse-backend/internal/logger"
	"wisdomHouse-backend/internal/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Database struct {
	*gorm.DB
}

// NewDatabase connects to Postgres using cfg.ConnectionString().
// Production must not run unsafe AutoMigrate automatically. Use RUN_AUTOMIGRATE=true
// only for a controlled migration job.
func NewDatabase(cfg *config.DatabaseConfig, appEnv string) (*Database, error) {
	dsn := cfg.ConnectionString()

	applog.L().Info("connecting to database", "dsn", cfg.DSN())

	gormLogger := logger.Default
	if isProduction(appEnv) {
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

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}
	applog.L().Info("database connection established")

	if err := ensureExtensions(db, appEnv); err != nil {
		return nil, fmt.Errorf("failed to ensure database extensions: %w", err)
	}

	if err := ensureDatabaseCompatibility(db); err != nil {
		return nil, fmt.Errorf("failed to ensure database compatibility: %w", err)
	}

	if shouldRunAutoMigrate(appEnv) {
		if err := AutoMigrate(db); err != nil {
			return nil, fmt.Errorf("failed to run automigrate: %w", err)
		}
	} else {
		applog.L().Info("production mode: AutoMigrate disabled; run a controlled migration job with RUN_AUTOMIGRATE=true")
		if err := ensureAdminDomainSchema(db, appEnv); err != nil {
			return nil, fmt.Errorf("failed to ensure admin domain schema: %w", err)
		}
	}

	return &Database{db}, nil
}

func isProduction(appEnv string) bool {
	return strings.EqualFold(strings.TrimSpace(appEnv), "production")
}

func shouldRunAutoMigrate(appEnv string) bool {
	if !isProduction(appEnv) {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(os.Getenv("RUN_AUTOMIGRATE")), "true")
}

func shouldEnsureAdminSchemaOnStartup(appEnv string) bool {
	if !isProduction(appEnv) {
		return true
	}

	return strings.EqualFold(
		strings.TrimSpace(os.Getenv("ENSURE_ADMIN_SCHEMA_ON_STARTUP")),
		"true",
	)
}

func ensureExtensions(db *gorm.DB, appEnv string) error {
	applog.L().Info("ensuring required Postgres extensions")

	tryExec := func(sql string, name string) error {
		if err := db.Exec(sql).Error; err != nil {
			if isProduction(appEnv) {
				applog.L().Warn("could not ensure extension; enable it in Supabase if required", "extension", name, "error", err)
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

	applog.L().Info("extensions check done")
	return nil
}

// ensureDatabaseCompatibility only runs safe, idempotent SQL.
// It must not fail when tables or constraints are missing.
func ensureDatabaseCompatibility(db *gorm.DB) error {
	applog.L().Info("ensuring database compatibility fixes")

	statements := []string{
		`ALTER TABLE IF EXISTS "members" DROP CONSTRAINT IF EXISTS "uni_members_email";`,
		`ALTER TABLE IF EXISTS "workforce_members" DROP CONSTRAINT IF EXISTS "uni_workforce_members_email";`,
		`ALTER TABLE IF EXISTS "leadership_members" DROP CONSTRAINT IF EXISTS "uni_leadership_members_email";`,
	}

	for _, stmt := range statements {
		if err := db.Exec(stmt).Error; err != nil {
			return err
		}
	}

	applog.L().Info("database compatibility fixes done")
	return nil
}

func AutoMigrate(db *gorm.DB) error {
	applog.L().Info("running database AutoMigrate")

	if err := runAutoMigrate(db, allMigrationModels()...); err != nil {
		return err
	}

	applog.L().Info("AutoMigrate completed successfully")
	return nil
}

func ensureAdminDomainSchema(db *gorm.DB, appEnv string) error {
	if !shouldEnsureAdminSchemaOnStartup(appEnv) {
		applog.L().Info("production mode: admin startup schema AutoMigrate is disabled")
		return nil
	}

	applog.L().Info("ensuring critical admin domain tables")

	if err := runAutoMigrate(db, adminDomainMigrationModels()...); err != nil {
		return err
	}

	applog.L().Info("critical admin domain tables check completed")
	return nil
}

func runAutoMigrate(db *gorm.DB, modelsToMigrate ...any) error {
	for _, model := range modelsToMigrate {
		if err := db.AutoMigrate(model); err != nil {
			if isIgnorableMissingConstraintError(err) {
				applog.L().Warn("ignoring stale/missing constraint cleanup error while migrating", "model", fmt.Sprintf("%T", model), "error", err)
				continue
			}

			return fmt.Errorf("failed to auto-migrate %T: %w", model, err)
		}
	}

	return nil
}

func isIgnorableMissingConstraintError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())

	return strings.Contains(msg, "sqlstate 42704") &&
		strings.Contains(msg, "constraint") &&
		strings.Contains(msg, "does not exist")
}

func allMigrationModels() []any {
	return []any{
		&models.User{},
		&models.AdminNotification{},
		&models.ApprovalRequest{},
		&models.Testimonial{},
		&models.Event{},
		&models.Reel{},
		&models.Form{},
		&models.FormSlugAlias{},
		&models.FormField{},
		&models.FormSubmission{},
		&models.FormCalendarReminder{},
		&models.FormCampaignDelivery{},
		&models.AdminEmailDelivery{},
		&models.AdminEmailSchedule{},
		&models.AdminEmailScheduleRun{},
		&models.CelebrationAutomationConfig{},
		&models.CelebrationAutomationRun{},
		&models.CelebrationDelivery{},
		&models.WeddingAnniversary{},
		&models.RegistrationSequence{},
		&models.Asset{},
		&models.EmailTemplate{},
		&models.Subscriber{},
		&models.Notification{},
		&models.NotificationDelivery{},
		&models.OTP{},
		&models.TicketSequence{},
		&models.WorkforceMember{},
		&models.Member{},
		&models.LeadershipMember{},
		&models.SecurityEvent{},
		&models.TrustedDevice{},
		&models.AnalyticsBatch{},
		&models.AnalyticsEvent{},
		&models.StoreProduct{},
		&models.StoreOrder{},
		&models.StoreOrderItem{},
		&models.SiteContent{},
		&models.PastoralCareRequest{},
		&models.GivingIntent{},
		&models.ContactMessage{},
		&models.VisitRequest{},
		&models.VisitActivity{},
	}
}

func adminDomainMigrationModels() []any {
	return []any{
		&models.WorkforceMember{},
		&models.Member{},
		&models.LeadershipMember{},
		&models.Form{},
		&models.FormField{},
		&models.FormSubmission{},
		&models.FormCalendarReminder{},
		&models.FormCampaignDelivery{},
	}
}

func (d *Database) Close() error {
	sqlDB, err := d.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
