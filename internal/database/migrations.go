package database

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// MigrationRecord tracks which migrations have been applied
type MigrationRecord struct {
	ID        int    `gorm:"primaryKey"`
	Name      string `gorm:"uniqueIndex;not null"`
	AppliedAt int64  `gorm:"autoCreateTime:milli"`
}

// TableName specifies the table name for the migration record
func (MigrationRecord) TableName() string {
	return "schema_migrations"
}

// RunMigrations executes all pending migrations in order
func RunMigrations(db *gorm.DB, migrationsDir string) error {
	// Create migrations table if it doesn't exist
	if err := db.AutoMigrate(&MigrationRecord{}); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Get all migration files
	files, err := getMigrationFiles(migrationsDir)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		log.Println("No migration files found")
		return nil
	}

	// Apply each migration
	for _, file := range files {
		filename := filepath.Base(file)

		// Skip rollback files
		if strings.HasSuffix(filename, ".down.sql") {
			continue
		}

		// Check if migration was already applied
		var record MigrationRecord
		result := db.Where("name = ?", filename).First(&record)

		if result.Error == nil {
			log.Printf("✓ Migration already applied: %s", filename)
			continue
		}

		if result.Error != gorm.ErrRecordNotFound {
			return fmt.Errorf("failed to check migration status: %w", result.Error)
		}

		// Read and execute migration
		content, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", filename, err)
		}

		log.Printf("→ Running migration: %s", filename)

		if err := db.Exec(string(content)).Error; err != nil {
			return fmt.Errorf("failed to execute migration %s: %w", filename, err)
		}

		// Record that migration was applied
		if err := db.Create(&MigrationRecord{Name: filename}).Error; err != nil {
			return fmt.Errorf("failed to record migration %s: %w", filename, err)
		}

		log.Printf("✓ Migration completed: %s", filename)
	}

	log.Println("All migrations completed successfully")
	return nil
}

// getMigrationFiles returns sorted list of .up.sql migration files
func getMigrationFiles(migrationsDir string) ([]string, error) {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory: %w", err)
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".up.sql") {
			files = append(files, filepath.Join(migrationsDir, entry.Name()))
		}
	}

	// Special handling for schema.up.sql - run it first
	sort.Slice(files, func(i, j int) bool {
		iBase := filepath.Base(files[i])
		jBase := filepath.Base(files[j])

		// schema.up.sql always comes first
		if iBase == "schema.up.sql" {
			return true
		}
		if jBase == "schema.up.sql" {
			return false
		}

		// Others sorted alphabetically
		return iBase < jBase
	})

	return files, nil
}
