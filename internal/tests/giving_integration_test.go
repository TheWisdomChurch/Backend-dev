//go:build integration

package tests

import (
	"context"
	"testing"
	"time"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
	"wisdomHouse-backend/internal/testutil"
)

func TestGivingRepo_CreateAndFindCategory(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewGivingRepository(db)
	ctx := context.Background()

	cat := testutil.BuildGivingCategory(t, db.DB)

	cats, err := repo.ListCategories(ctx)
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}

	var found bool
	for _, c := range cats {
		if c.ID == cat.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("created category %s not returned by ListCategories", cat.ID)
	}
}

func TestGivingRepo_CreateTransaction_And_FindByRef(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewGivingRepository(db)
	ctx := context.Background()

	cat := testutil.BuildGivingCategory(t, db.DB)

	tx := &models.GivingTransaction{
		CategoryID:      cat.ID,
		AmountKobo:      100_000, // ₦1,000
		Currency:        "NGN",
		Channel:         "cash",
		PaymentRef:      "MANUAL-TEST-001",
		PaymentProvider: "manual",
		Status:          "success",
		GiverName:       "John Doe",
		GiverEmail:      "john.doe@test.example",
		GivenAt:         time.Now().UTC(),
	}

	if err := repo.Create(ctx, tx); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tx.ID == "" {
		t.Fatal("expected non-empty ID after Create")
	}

	got, err := repo.FindByRef(ctx, "MANUAL-TEST-001")
	if err != nil {
		t.Fatalf("FindByRef: %v", err)
	}
	if got.AmountKobo != 100_000 {
		t.Errorf("AmountKobo: want 100000, got %d", got.AmountKobo)
	}
}

func TestGivingRepo_UpdateStatus(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewGivingRepository(db)
	ctx := context.Background()

	cat := testutil.BuildGivingCategory(t, db.DB)

	tx := &models.GivingTransaction{
		CategoryID:  cat.ID,
		AmountKobo:  50_000,
		Currency:    "NGN",
		Channel:     "card",
		PaymentRef:  "PENDING-REF-001",
		Status:      "pending",
		GiverEmail:  "donor@test.example",
		GivenAt:     time.Now().UTC(),
	}
	if err := repo.Create(ctx, tx); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.UpdateStatus(ctx, tx.ID, "success"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	updated, err := repo.FindByRef(ctx, "PENDING-REF-001")
	if err != nil {
		t.Fatalf("FindByRef after update: %v", err)
	}
	if updated.Status != "success" {
		t.Errorf("Status after update: want %q, got %q", "success", updated.Status)
	}
}

func TestGivingRepo_List_Pagination(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewGivingRepository(db)
	ctx := context.Background()

	cat := testutil.BuildGivingCategory(t, db.DB)

	for i := 0; i < 5; i++ {
		_ = repo.Create(ctx, &models.GivingTransaction{
			CategoryID: cat.ID,
			AmountKobo: int64(10_000 * (i + 1)),
			Currency:   "NGN",
			Channel:    "transfer",
			PaymentRef: "LIST-REF-" + string(rune('A'+i)),
			Status:     "success",
			GivenAt:    time.Now().UTC(),
		})
	}

	items, total, err := repo.List(ctx, repository.GivingFilter{}, 3, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total < 5 {
		t.Errorf("List total: want ≥5, got %d", total)
	}
	if len(items) > 3 {
		t.Errorf("List returned more than limit=3: %d", len(items))
	}
}
