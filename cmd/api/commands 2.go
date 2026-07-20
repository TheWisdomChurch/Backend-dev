package main

import (
	"os"

	"wisdomHouse-backend/internal/config"
	"wisdomHouse-backend/internal/database"
	applog "wisdomHouse-backend/internal/logger"
)

// runMigrateCommand handles the `migrate` subcommand: connect to the database, run
// pending migrations, and exit — without booting the HTTP server, workers, or any
// other subsystem. Invoked via `make migrate` / `go run cmd/api/main.go migrate`.
func runMigrateCommand(cfg *config.Config) {
	db, err := database.NewDatabase(&cfg.Database, cfg.App.Environment)
	if err != nil {
		applog.L().Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(); err != nil {
			applog.L().Warn("error closing database", "error", err)
		}
	}()

	if err := verifyDatabaseConnection(db); err != nil {
		applog.L().Error("database connection failed", "error", err)
		os.Exit(1)
	}

	if err := database.RunMigrations(db.DB, "migrations"); err != nil {
		applog.L().Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	applog.L().Info("migrations complete")
}
