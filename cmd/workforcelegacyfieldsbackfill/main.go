// One-time maintenance script: backfills birthday/anniversary specifically
// for the "Workforce Service Profile" form, whose fields were built with
// auto-generated keys (field_6, field_7, field_8, field_9) instead of
// semantic ones — so buildWorkforceRequest's key-based lookup could never
// have matched them; there was nothing named "birthday" to find.
//
// Confirmed from a real production dump (cmd/inspectworkforcesubmissions):
//
//	field_6 = marital status ("single"/"married")
//	field_7 = wedding anniversary (DD-MM) — present only when field_6=married
//	field_8 = date of birth (DD-MM) — present on every submission
//	field_9 = free-text ("why serve") — not a date, left alone
//
// This backfill is intentionally narrow (this one form's known layout),
// not a generic fix — the durable fix is renaming those fields to
// "birthday"/"anniversary" in the Forms builder so future submissions flow
// through the normal, semantic field mapping without needing this at all.
//
// Safe by default: DRY_RUN=false must be set explicitly to write anything.
// Idempotent: only fills in whichever of birthday/anniversary a member is
// still missing, never overwrites existing data.
//
// Usage:
//
//	DATABASE_URL=... go run ./cmd/workforcelegacyfieldsbackfill
//	DATABASE_URL=... DRY_RUN=false go run ./cmd/workforcelegacyfieldsbackfill
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

func stringValue(values map[string]any, key string) string {
	raw, ok := values[key]
	if !ok || raw == nil {
		return ""
	}
	if s, ok := raw.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprint(raw))
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
		if submission.FormTitle != "Workforce Service Profile" {
			continue
		}

		submissionEmail := ""
		if submission.Email != nil {
			submissionEmail = strings.ToLower(strings.TrimSpace(*submission.Email))
		}
		if submissionEmail == "" {
			skippedNoEmail++
			continue
		}

		member, err := workforceRepo.GetByEmail(submissionEmail)
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

		birthday := stringValue(values, "field_8")
		anniversary := stringValue(values, "field_7")

		update := &models.UpdateWorkforceRequest{}
		gotNewData := false
		if missingBirthday && birthday != "" {
			update.Birthday = &birthday
			gotNewData = true
		}
		if missingAnniversary && anniversary != "" {
			update.Anniversary = &anniversary
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
