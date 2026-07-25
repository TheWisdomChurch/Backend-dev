// One-time maintenance script: promotes a single admin user to the
// super_admin role by email. Needed because the approval workflows (event
// delete, leadership delete, workforce delete/registration) are
// deliberately gated to super_admin only — an account that's currently
// "admin" gets a 403 on those actions until promoted.
//
// Safe by default: DRY_RUN=false must be set explicitly to write anything.
// Refuses to do anything if the account is already super_admin, or if no
// account matches the given email.
//
// Usage:
//
//	DATABASE_URL=... EMAIL=someone@example.org go run ./cmd/promotesuperadmin
//	DATABASE_URL=... EMAIL=someone@example.org DRY_RUN=false go run ./cmd/promotesuperadmin
package main

import (
	"fmt"
	"os"
	"strings"

	"wisdomHouse-backend/internal/config"
	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/repository"
)

func main() {
	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dbURL == "" {
		fmt.Println("DATABASE_URL is required")
		os.Exit(1)
	}
	targetEmail := strings.TrimSpace(strings.ToLower(os.Getenv("EMAIL")))
	if targetEmail == "" {
		fmt.Println("EMAIL is required (the account to promote)")
		os.Exit(1)
	}
	dryRun := strings.TrimSpace(os.Getenv("DRY_RUN")) != "false"

	db, err := database.NewDatabase(&config.DatabaseConfig{URL: dbURL}, "production")
	if err != nil {
		fmt.Printf("failed to connect to database: %v\n", err)
		os.Exit(1)
	}

	userRepo := repository.NewUserRepository(db)

	user, err := userRepo.FindByEmail(targetEmail)
	if err != nil {
		fmt.Printf("failed to look up %s: %v\n", targetEmail, err)
		os.Exit(1)
	}
	if user == nil {
		fmt.Printf("no account found for %s\n", targetEmail)
		os.Exit(1)
	}

	fmt.Printf("found account: %s %s <%s> — current role: %s\n", user.FirstName, user.LastName, user.Email, user.Role)

	if strings.EqualFold(user.Role, "super_admin") {
		fmt.Println("already super_admin — nothing to do")
		return
	}

	if dryRun {
		fmt.Printf("would change role %q -> \"super_admin\" for %s\n", user.Role, user.Email)
		fmt.Println("DRY RUN — set DRY_RUN=false to actually apply this")
		return
	}

	user.Role = "super_admin"
	if err := userRepo.Update(user); err != nil {
		fmt.Printf("FAILED to update role: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("done — %s is now super_admin\n", user.Email)
}
