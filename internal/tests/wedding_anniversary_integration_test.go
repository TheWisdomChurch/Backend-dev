//go:build integration

package tests

import (
	"context"
	"testing"

	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/testutil"
)

func newWeddingAnniversarySvc(repo repository.WeddingAnniversaryRepository) service.WeddingAnniversaryService {
	return service.NewWeddingAnniversaryService(repo, nil, nil, nil, email.Branding{PublicURL: "https://api.test.example"}, "test-secret-test-secret-32-bytes!")
}

func TestWeddingAnniversaryRepo_UpsertAndFetch(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewWeddingAnniversaryRepository(db)
	m := testutil.BuildMember(t, db.DB)

	ctx := context.Background()
	row, err := repo.Upsert(ctx, &models.WeddingAnniversary{
		SubjectType:      models.WeddingAnniversarySubjectMember,
		SubjectID:        m.ID,
		AnniversaryMonth: 6,
		AnniversaryDay:   14,
		SpouseName:       "Sarah",
		Source:           models.WeddingAnniversarySourceAdmin,
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if row.SpouseName != "Sarah" {
		t.Fatalf("spouse name: got %q", row.SpouseName)
	}

	got, err := repo.GetBySubject(ctx, "member", m.ID)
	if err != nil {
		t.Fatalf("GetBySubject: %v", err)
	}
	if got.AnniversaryMonth != 6 || got.AnniversaryDay != 14 {
		t.Fatalf("unexpected date: %+v", got)
	}
}

func TestWeddingAnniversaryRepo_MirrorCaseMerges(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewWeddingAnniversaryRepository(db)
	ctx := context.Background()

	david := testutil.BuildMember(t, db.DB)
	sarah := testutil.BuildMember(t, db.DB)

	// David's row links Sarah as his spouse.
	if _, err := repo.Upsert(ctx, &models.WeddingAnniversary{
		SubjectType: models.WeddingAnniversarySubjectMember, SubjectID: david.ID,
		AnniversaryMonth: 6, AnniversaryDay: 14, SpouseName: "Sarah",
		SpouseSubjectType: strPtr("member"), SpouseSubjectID: &sarah.ID,
		Source: models.WeddingAnniversarySourceAdmin,
	}); err != nil {
		t.Fatalf("upsert david: %v", err)
	}

	// A later write for Sarah, linking back to David, must merge into the
	// same row rather than creating a second marriage record.
	if _, err := repo.Upsert(ctx, &models.WeddingAnniversary{
		SubjectType: models.WeddingAnniversarySubjectMember, SubjectID: sarah.ID,
		AnniversaryMonth: 6, AnniversaryDay: 14, SpouseName: "David",
		SpouseSubjectType: strPtr("member"), SpouseSubjectID: &david.ID,
		Source: models.WeddingAnniversarySourceForm,
	}); err != nil {
		t.Fatalf("upsert sarah: %v", err)
	}

	rows, total, err := repo.List(ctx, repository.WeddingAnniversaryFilter{Limit: 50})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected exactly one marriage row after merge, got %d (%+v)", total, rows)
	}
}

func TestWeddingAnniversaryRepo_ListDueByMonthDay(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewWeddingAnniversaryRepository(db)
	ctx := context.Background()

	active := testutil.BuildMember(t, db.DB)
	archived := testutil.BuildMember(t, db.DB)

	if _, err := repo.Upsert(ctx, &models.WeddingAnniversary{
		SubjectType: models.WeddingAnniversarySubjectMember, SubjectID: active.ID,
		AnniversaryMonth: 3, AnniversaryDay: 9, SpouseName: "Grace", Source: models.WeddingAnniversarySourceAdmin,
	}); err != nil {
		t.Fatalf("upsert active: %v", err)
	}
	archivedRow, err := repo.Upsert(ctx, &models.WeddingAnniversary{
		SubjectType: models.WeddingAnniversarySubjectMember, SubjectID: archived.ID,
		AnniversaryMonth: 3, AnniversaryDay: 9, SpouseName: "Ruth", Source: models.WeddingAnniversarySourceAdmin,
	})
	if err != nil {
		t.Fatalf("upsert archived: %v", err)
	}
	if err := repo.SetStatus(ctx, archivedRow.ID, "archived"); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}

	due, err := repo.ListDueByMonthDay(ctx, 3, 9)
	if err != nil {
		t.Fatalf("ListDueByMonthDay: %v", err)
	}
	if len(due) != 1 || due[0].SubjectID != active.ID {
		t.Fatalf("expected only the active row due, got %+v", due)
	}
}

func TestWeddingAnniversaryService_ConflictKeepsAdminDate(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewWeddingAnniversaryRepository(db)
	svc := newWeddingAnniversarySvc(repo)
	ctx := context.Background()

	m := testutil.BuildMember(t, db.DB)
	date1 := "14/06"
	if _, err := svc.UpsertForSubject(ctx, "member", m.ID, models.WeddingAnniversaryInput{
		Anniversary: &date1, SpouseName: "sarah",
	}, models.WeddingAnniversarySourceAdmin, nil); err != nil {
		t.Fatalf("initial admin upsert: %v", err)
	}

	date2 := "01/01"
	if _, err := svc.UpsertForSubject(ctx, "member", m.ID, models.WeddingAnniversaryInput{
		Anniversary: &date2, SpouseName: "sarah",
	}, models.WeddingAnniversarySourceForm, nil); err != nil {
		t.Fatalf("conflicting form upsert: %v", err)
	}

	stored, getErr := repo.GetBySubject(ctx, "member", m.ID)
	if getErr != nil {
		t.Fatalf("GetBySubject: %v", getErr)
	}
	if stored.AnniversaryMonth != 6 || stored.AnniversaryDay != 14 {
		t.Fatalf("admin-entered date must survive a conflicting form write, got %02d/%02d", stored.AnniversaryDay, stored.AnniversaryMonth)
	}
}

func strPtr(s string) *string { return &s }
