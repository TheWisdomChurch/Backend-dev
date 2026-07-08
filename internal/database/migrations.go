package database

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gorm.io/gorm"

	applog "wisdomHouse-backend/internal/logger"
)

// MigrationRecord tracks which migrations have been applied.
//
// Table name is deliberately not "schema_migrations" — that name collides with the
// table golang-migrate/migrate uses (different, incompatible column layout). See
// docs/MIGRATIONS.md.
type MigrationRecord struct {
	ID        int    `gorm:"primaryKey"`
	Name      string `gorm:"uniqueIndex;not null"`
	AppliedAt int64  `gorm:"autoCreateTime:milli"`
}

// TableName specifies the table name for the migration record
func (MigrationRecord) TableName() string {
	return "app_schema_migrations"
}

// RunMigrations executes all pending migrations in order
func RunMigrations(db *gorm.DB, migrationsDir string) error {
	if err := renameLegacyMigrationsTable(db); err != nil {
		return fmt.Errorf("failed to migrate legacy migrations table: %w", err)
	}

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
		applog.L().Info("no migration files found")
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
			applog.L().Debug("migration already applied", "file", filename)
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

		applog.L().Info("running migration", "file", filename)

		// Wrap execution + bookkeeping in a transaction so a failing migration never
		// leaves the schema half-applied while also being unrecorded (which would
		// otherwise re-run on next boot against a partially-mutated schema).
		txErr := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(string(content)).Error; err != nil {
				return fmt.Errorf("failed to execute migration %s: %w", filename, err)
			}
			if err := tx.Create(&MigrationRecord{Name: filename}).Error; err != nil {
				return fmt.Errorf("failed to record migration %s: %w", filename, err)
			}
			return nil
		})
		if txErr != nil {
			return txErr
		}

		applog.L().Info("migration completed", "file", filename)
	}

	applog.L().Info("all migrations completed successfully")
	return nil
}

// renameLegacyMigrationsTable is a one-time upgrade step: earlier versions of this
// runner tracked applied migrations in a table literally named "schema_migrations",
// which collides with the table golang-migrate/migrate owns. If that legacy table
// exists and has our column shape (not golang-migrate's), rename it in place so
// migration history isn't lost and nothing re-runs.
func renameLegacyMigrationsTable(db *gorm.DB) error {
	var newTableExists bool
	if err := db.Raw(`SELECT EXISTS (
		SELECT 1 FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = ?
	)`, MigrationRecord{}.TableName()).Scan(&newTableExists).Error; err != nil {
		return err
	}
	if newTableExists {
		return nil
	}

	var isOurLegacyTable bool
	if err := db.Raw(`SELECT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'schema_migrations' AND column_name = 'applied_at'
	)`).Scan(&isOurLegacyTable).Error; err != nil {
		return err
	}
	if !isOurLegacyTable {
		// Either no legacy table at all, or it's golang-migrate's (version/dirty) — leave it untouched.
		return nil
	}

	newTable := MigrationRecord{}.TableName()
	applog.L().Info("renaming legacy migrations table", "from", "schema_migrations", "to", newTable)
	if err := db.Exec(fmt.Sprintf(`ALTER TABLE schema_migrations RENAME TO %s`, newTable)).Error; err != nil {
		return err
	}

	// Postgres RENAME TABLE does not rename constraint/index names along with it,
	// so the old table's unique constraint on "name" keeps its original
	// schema_migrations-based name. The subsequent AutoMigrate call expects a
	// constraint name derived from the *new* table name and will fail trying to
	// drop one that was never there. Drop the old-named unique constraint/index
	// here so AutoMigrate can recreate it fresh, correctly named.
	return dropUniqueConstraints(db, newTable)
}

// dropUniqueConstraints drops every unique constraint or unique index on the
// given table that isn't the primary key, regardless of its name — used so a
// later AutoMigrate can recreate uniqueness constraints under GORM's expected
// naming without tripping over stale names left behind by a table rename.
func dropUniqueConstraints(db *gorm.DB, table string) error {
	var constraintNames []string
	if err := db.Raw(`
		SELECT con.conname
		FROM pg_constraint con
		JOIN pg_class rel ON rel.oid = con.conrelid
		WHERE rel.relname = ? AND con.contype = 'u'
	`, table).Scan(&constraintNames).Error; err != nil {
		return fmt.Errorf("inspecting unique constraints on %s: %w", table, err)
	}
	for _, name := range constraintNames {
		if err := db.Exec(fmt.Sprintf(`ALTER TABLE %s DROP CONSTRAINT %s`, table, name)).Error; err != nil {
			return fmt.Errorf("dropping constraint %s on %s: %w", name, table, err)
		}
	}

	var indexNames []string
	if err := db.Raw(`
		SELECT indexname FROM pg_indexes
		WHERE tablename = ? AND indexdef ILIKE '%UNIQUE%' AND indexname NOT LIKE '%pkey%'
	`, table).Scan(&indexNames).Error; err != nil {
		return fmt.Errorf("inspecting unique indexes on %s: %w", table, err)
	}
	for _, name := range indexNames {
		if err := db.Exec(fmt.Sprintf(`DROP INDEX IF EXISTS %s`, name)).Error; err != nil {
			return fmt.Errorf("dropping index %s on %s: %w", name, table, err)
		}
	}

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
