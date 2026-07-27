// Read-only diagnostic: for every leadership_delete approval request marked
// approved, checks whether the leadership_members row it was supposed to
// remove is actually gone, and if not, whether a still-active (not
// soft-deleted) intake form submission exists for that person's email —
// which would explain a resurrection via syncLeadershipIntakeSubmissions.
// Changes nothing.
//
// Usage:
//
//	DATABASE_URL=... go run ./cmd/inspectleadershipdeletes
package main

import (
	"fmt"
	"os"
	"strings"

	"wisdomHouse-backend/internal/config"
	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

func main() {
	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dbURL == "" {
		fmt.Println("DATABASE_URL is required")
		os.Exit(1)
	}

	db, err := database.NewDatabase(&config.DatabaseConfig{URL: dbURL}, "production")
	if err != nil {
		fmt.Printf("failed to connect to database: %v\n", err)
		os.Exit(1)
	}

	approvalRepo := repository.NewApprovalRequestRepository(db)
	leadershipRepo := repository.NewLeadershipRepository(db)

	requests, err := approvalRepo.List(
		[]models.ApprovalRequestType{models.ApprovalTypeLeadershipDelete},
		[]models.ApprovalRequestStatus{models.ApprovalStatusApproved},
		nil, nil, 500,
	)
	if err != nil {
		fmt.Printf("failed to list leadership_delete requests: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("found %d approved leadership_delete request(s)\n\n", len(requests))

	stillPresent := 0
	for _, req := range requests {
		entityID := ""
		if req.EntityID != nil {
			entityID = *req.EntityID
		}
		label := ""
		if req.EntityLabel != nil {
			label = *req.EntityLabel
		}

		member, err := leadershipRepo.GetByID(entityID)
		if err != nil || member == nil {
			fmt.Printf("OK    %-30s ticket=%s entity=%s — actually gone\n", label, req.TicketCode, entityID)
			continue
		}

		stillPresent++
		fmt.Printf("BUG   %-30s ticket=%s entity=%s — approved %v ago but STILL in leadership_members (email=%v, status=%v)\n",
			label, req.TicketCode, entityID, req.ApprovedAt, memberEmail(member), member.Status)

		if member.Email != nil {
			var count int64
			cleanEmail := strings.ToLower(strings.TrimSpace(*member.Email))
			db.DB.Raw(`
				SELECT count(*) FROM form_submissions fs
				JOIN forms f ON f.id = fs.form_id
				WHERE fs.deleted_at IS NULL
				  AND LOWER(fs.email) = ?
				  AND (
				    LOWER(COALESCE(f.settings->>'submissionTarget', '')) = 'leadership'
				    OR LOWER(COALESCE(f.settings->>'formType', '')) = 'leadership'
				    OR LOWER(COALESCE(f.slug, '')) LIKE '%leadership%'
				    OR LOWER(COALESCE(f.title, '')) LIKE '%leadership%'
				  )
			`, cleanEmail).Scan(&count)
			fmt.Printf("      -> %d still-active leadership intake submission(s) for %s (deleted_at IS NULL)\n", count, cleanEmail)
		}
	}

	fmt.Printf("\ndone — %d/%d approved leadership_delete tickets have a member that's still present\n", stillPresent, len(requests))
}

func memberEmail(m *models.LeadershipMember) string {
	if m == nil || m.Email == nil {
		return ""
	}
	return *m.Email
}
