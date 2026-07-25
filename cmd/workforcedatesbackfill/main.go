// One-time maintenance script: backfills birthday/anniversary on existing
// workforce members whose original public-form submission had that data,
// but whose record was created before the field-mapping fix (the old
// mapper only recognized "birthday"/"birthDate"/"dob" and never looked for
// an anniversary at all, so a form asking "Date of Birth" or "Wedding
// Anniversary" under those labels silently dropped both).
//
// Matches submissions to members by email, re-parses the submission's raw
// values with the corrected field mapping, and only fills in whichever of
// birthday/anniversary the member is still missing — never overwrites data
// that's already there (including anything an admin has since corrected
// by hand).
//
// Safe by default: DRY_RUN=false must be set explicitly to write anything.
// Idempotent: re-running only ever touches members still missing a date.
//
// Usage:
//
//	DATABASE_URL=... go run ./cmd/workforcedatesbackfill
//	DATABASE_URL=... DRY_RUN=false go run ./cmd/workforcedatesbackfill
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"wisdomHouse-backend/internal/config"
	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
	"wisdomHouse-backend/internal/service"
)

func strPtrOr(v *string, fallback string) string {
	if v == nil {
		return fallback
	}
	return *v
}

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

	formRepo := repository.NewFormRepository(db)
	workforceRepo := repository.NewWorkforceRepository(db)
	workforceSvc := service.NewWorkforceService(workforceRepo, nil, nil, nil, email.Branding{})

	submissions, err := formRepo.ListWorkforceIntakeSubmissions(1000)
	if err != nil {
		fmt.Printf("failed to list workforce intake submissions: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("found %d workforce intake submission(s) to check\n", len(submissions))

	if dryRun {
		fmt.Println("DRY RUN — set DRY_RUN=false to actually write updates")
	}

	updated, skippedNoEmail, skippedNoMember, skippedNothingMissing, skippedNoNewData, failed := 0, 0, 0, 0, 0, 0

	for _, submission := range submissions {
		email := ""
		if submission.Email != nil {
			email = strings.ToLower(strings.TrimSpace(*submission.Email))
		}
		if email == "" {
			skippedNoEmail++
			continue
		}

		member, err := workforceRepo.GetByEmail(email)
		if err != nil || member == nil {
			skippedNoMember++
			continue
		}

		missingBirthday := member.BirthdayMonth == nil || member.BirthdayDay == nil
		missingAnniversary := member.AnniversaryMonth == nil || member.AnniversaryDay == nil
		if !missingBirthday && !missingAnniversary {
			skippedNothingMissing++
			continue
		}

		values := map[string]any{}
		if len(submission.Values) > 0 {
			if err := json.Unmarshal(submission.Values, &values); err != nil {
				failed++
				fmt.Printf("FAILED to parse submission %s values: %v\n", submission.ID, err)
				continue
			}
		}

		mapped, err := service.BuildWorkforceRequestFromValues(values)
		if err != nil {
			// Missing firstName/lastName/department etc. — not this
			// script's concern, it only cares about the date fields.
			continue
		}

		update := &models.UpdateWorkforceRequest{}
		gotNewData := false
		if missingBirthday && mapped.Birthday != nil && strings.TrimSpace(*mapped.Birthday) != "" {
			update.Birthday = mapped.Birthday
			gotNewData = true
		}
		if missingAnniversary && mapped.Anniversary != nil && strings.TrimSpace(*mapped.Anniversary) != "" {
			update.Anniversary = mapped.Anniversary
			gotNewData = true
		}
		if !gotNewData {
			skippedNoNewData++
			continue
		}

		name := strings.TrimSpace(strings.Join([]string{member.FirstName, member.LastName}, " "))
		summary := fmt.Sprintf("birthday=%s anniversary=%s", strPtrOr(update.Birthday, "-"), strPtrOr(update.Anniversary, "-"))
		if dryRun {
			fmt.Printf("would update %s (%s): %s\n", name, member.ID, summary)
			updated++
			continue
		}

		if _, err := workforceSvc.Update(member.ID, update); err != nil {
			fmt.Printf("FAILED to update %s (%s): %v\n", name, member.ID, err)
			failed++
			continue
		}
		fmt.Printf("updated %s (%s): %s\n", name, member.ID, summary)
		updated++
	}

	fmt.Printf(
		"\ndone — updated=%d skipped(no-email)=%d skipped(no-matching-member)=%d skipped(nothing-missing)=%d skipped(no-new-data)=%d failed=%d\n",
		updated, skippedNoEmail, skippedNoMember, skippedNothingMissing, skippedNoNewData, failed,
	)
}
