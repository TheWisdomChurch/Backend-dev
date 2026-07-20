// One-time maintenance script: raises a super-admin approval ticket
// (workforce_registration) for every workforce member that's already
// sitting in "pending"/"new" status. This backfills the ticket-queue gap —
// before this session's fix, pending applications had no path into the
// Requests queue at all, so a real backlog accumulated silently.
//
// Safe by default: DRY_RUN=false must be set explicitly to write anything.
// Idempotent: skips any member that already has a pending
// workforce_registration ticket, so it can be re-run safely.
//
// Usage:
//
//	DATABASE_URL=... go run ./cmd/workforcebacklog            # dry run, prints what it would do
//	DATABASE_URL=... DRY_RUN=false go run ./cmd/workforcebacklog   # actually creates tickets
package main

import (
	"fmt"
	"os"
	"strings"

	"wisdomHouse-backend/internal/config"
	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
	"wisdomHouse-backend/internal/service"
)

func main() {
	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dbURL == "" {
		fmt.Println("DATABASE_URL is required")
		os.Exit(1)
	}
	dryRun := strings.TrimSpace(os.Getenv("DRY_RUN")) != "false"

	db, err := database.NewDatabase(&config.DatabaseConfig{URL: dbURL}, "production")
	if err != nil {
		fmt.Printf("failed to connect to database: %v\n", err)
		os.Exit(1)
	}

	workforceRepo := repository.NewWorkforceRepository(db)
	approvalRepo := repository.NewApprovalRequestRepository(db)
	ticketSeqRepo := repository.NewTicketSequenceRepository(db)
	approvalSvc := service.NewApprovalService(approvalRepo, ticketSeqRepo)

	var pending []models.WorkforceMember
	for _, status := range []string{"pending", "new"} {
		items, _, err := workforceRepo.List(0, 5000, "", status)
		if err != nil {
			fmt.Printf("failed to list workforce members with status=%s: %v\n", status, err)
			os.Exit(1)
		}
		pending = append(pending, items...)
	}

	fmt.Printf("found %d workforce member(s) awaiting approval\n", len(pending))
	if dryRun {
		fmt.Println("DRY RUN — set DRY_RUN=false to actually create tickets")
	}

	created, skipped, failed := 0, 0, 0
	for _, member := range pending {
		if existing, err := approvalRepo.FindByEntity(models.ApprovalTypeWorkforceRegistration, member.ID); err == nil && existing != nil && existing.Status == models.ApprovalStatusPending {
			skipped++
			continue
		}

		label := strings.TrimSpace(strings.Join([]string{member.FirstName, member.LastName}, " "))
		if label == "" {
			label = member.ID
		}

		if dryRun {
			fmt.Printf("would create ticket for %s (%s, %s)\n", label, member.Department, member.ID)
			created++
			continue
		}

		if _, err := approvalSvc.CreateRequest(service.CreateApprovalRequest{
			Type:        models.ApprovalTypeWorkforceRegistration,
			EntityID:    &member.ID,
			EntityLabel: &label,
		}); err != nil {
			fmt.Printf("FAILED to create ticket for %s (%s): %v\n", label, member.ID, err)
			failed++
			continue
		}

		fmt.Printf("created ticket for %s (%s, %s)\n", label, member.Department, member.ID)
		created++
	}

	fmt.Printf("\ndone — created=%d skipped(already ticketed)=%d failed=%d\n", created, skipped, failed)
}
