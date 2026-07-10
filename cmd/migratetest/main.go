package main

import (
	"fmt"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"wisdomHouse-backend/internal/database"
)

func main() {
	dbname := os.Getenv("TEST_DB")
	if dbname == "" {
		dbname = "wisdom_audit_test"
	}
	dsn := "host=/tmp user=TechDev dbname=" + dbname + " sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		fmt.Println("connect error:", err)
		os.Exit(1)
	}

	if err := database.RunMigrations(db, "migrations"); err != nil {
		fmt.Println("migration error:", err)
		os.Exit(1)
	}

	fmt.Println("MIGRATIONS_OK")
}
