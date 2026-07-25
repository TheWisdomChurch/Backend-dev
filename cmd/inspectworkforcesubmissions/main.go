// Read-only diagnostic: prints the raw field keys (and values) submitted
// on each workforce intake form submission, so the date-field mapping in
// buildWorkforceRequest can be corrected against what the form actually
// sends instead of guessed at. Does not modify anything.
//
// Usage:
//
//	DATABASE_URL=... go run ./cmd/inspectworkforcesubmissions
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
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

	db, err := database.NewDatabase(&config.DatabaseConfig{URL: dbURL}, "production")
	if err != nil {
		fmt.Printf("failed to connect to database: %v\n", err)
		os.Exit(1)
	}

	formRepo := repository.NewFormRepository(db)
	submissions, err := formRepo.ListWorkforceIntakeSubmissions(1000)
	if err != nil {
		fmt.Printf("failed to list workforce intake submissions: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("found %d workforce intake submission(s)\n\n", len(submissions))

	keyCounts := map[string]int{}
	shown := 0

	for _, submission := range submissions {
		values := map[string]any{}
		if len(submission.Values) > 0 {
			if err := json.Unmarshal(submission.Values, &values); err != nil {
				continue
			}
		}

		keys := make([]string, 0, len(values))
		for k := range values {
			keys = append(keys, k)
			keyCounts[k]++
		}
		sort.Strings(keys)

		if shown < 5 {
			fmt.Printf("--- submission %s (form: %s) ---\n", submission.ID, submission.FormTitle)
			for _, k := range keys {
				fmt.Printf("  %-25s = %v\n", k, values[k])
			}
			fmt.Println()
			shown++
		}
	}

	fmt.Println("=== field key frequency across all submissions ===")
	type kv struct {
		key   string
		count int
	}
	sorted := make([]kv, 0, len(keyCounts))
	for k, c := range keyCounts {
		sorted = append(sorted, kv{k, c})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].count > sorted[j].count })
	for _, item := range sorted {
		fmt.Printf("  %-25s %d\n", item.key, item.count)
	}
}
