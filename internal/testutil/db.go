package testutil

import (
	"os"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"wisdomHouse-backend/internal/database"
)

// NewTestDB opens a GORM connection using TEST_DATABASE_URL, begins a
// transaction, and returns a *database.Database scoped to that transaction.
// The transaction is rolled back in t.Cleanup — every test gets a clean slate
// without touching committed data or needing a separate test schema.
//
// Tests that call this function are skipped when TEST_DATABASE_URL is unset,
// so `go test ./...` never fails in CI environments without a database.
func NewTestDB(t *testing.T) *database.Database {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("testutil.NewTestDB: open: %v", err)
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("testutil.NewTestDB: begin tx: %v", tx.Error)
	}

	t.Cleanup(func() {
		tx.Rollback()
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})

	return &database.Database{DB: tx}
}
