//go:build integration

package tests

import (
	"testing"

	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/repository"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/testutil"
)

// newMemberSvc constructs a memberService with only the repo wired up.
// nil is safe for formRepo / eventRepo / sender because BulkImport does not use them.
func newMemberSvc(repo repository.MemberRepository) service.MemberService {
	return service.NewMemberService(repo, nil, nil, nil, email.Branding{}, "")
}

func TestMemberRepo_Create(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewMemberRepository(db)

	m := testutil.BuildMember(t, db.DB)
	got, err := repo.GetByID(m.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Email != m.Email {
		t.Errorf("email: want %q, got %q", m.Email, got.Email)
	}
}

func TestMemberRepo_List_ActiveFilter(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewMemberRepository(db)

	testutil.BuildMember(t, db.DB)
	testutil.BuildMember(t, db.DB, testutil.WithMemberInactive())

	active := true
	items, total, err := repo.List(0, 100, &active)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total < 1 {
		t.Errorf("expected ≥1 active member, got %d", total)
	}
	for _, item := range items {
		if !item.IsActive {
			t.Errorf("List(active=true) returned inactive member %s", item.ID)
		}
	}
}

func TestMemberService_BulkImport(t *testing.T) {
	db := testutil.NewTestDB(t)
	svc := newMemberSvc(repository.NewMemberRepository(db))

	rows := []service.CSVMemberRow{
		{FirstName: "Bob", LastName: "Jones", Email: "bob.jones.bulk@test.example"},
		{FirstName: "Carol", LastName: "Lee", Email: "carol.lee.bulk@test.example", Birthday: "15/03"},
		{FirstName: "", LastName: "NoFirst", Email: "skip@test.example"}, // skipped — missing first_name
	}

	result, err := svc.BulkImport(rows)
	if err != nil {
		t.Fatalf("BulkImport: %v", err)
	}

	tests := []struct {
		field string
		want  int
		got   int
	}{
		{"Total", 3, result.Total},
		{"Imported", 2, result.Imported},
		{"Skipped", 1, result.Skipped},
	}
	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("BulkImport.%s: want %d, got %d", tc.field, tc.want, tc.got)
		}
	}
	if len(result.Errors) != 1 {
		t.Errorf("BulkImport.Errors len: want 1, got %d", len(result.Errors))
	}
}

func TestMemberService_BulkImport_BirthdayParsing(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewMemberRepository(db)
	svc := newMemberSvc(repo)

	rows := []service.CSVMemberRow{
		{FirstName: "Dave", LastName: "Bday", Email: "dave.bday@test.example", Birthday: "25/12"},
	}
	result, err := svc.BulkImport(rows)
	if err != nil {
		t.Fatalf("BulkImport: %v", err)
	}
	if result.Imported != 1 {
		t.Fatalf("expected 1 imported, got %d", result.Imported)
	}

	// Verify birthday was persisted correctly.
	members, _, _ := repo.List(0, 100, nil)
	var found bool
	for _, m := range members {
		if m.Email == "dave.bday@test.example" {
			found = true
			if m.BirthdayMonth == nil || *m.BirthdayMonth != 12 {
				t.Errorf("BirthdayMonth: want 12, got %v", m.BirthdayMonth)
			}
			if m.BirthdayDay == nil || *m.BirthdayDay != 25 {
				t.Errorf("BirthdayDay: want 25, got %v", m.BirthdayDay)
			}
		}
	}
	if !found {
		t.Error("imported member not found in repo")
	}
}

func TestMemberRepo_Stats(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewMemberRepository(db)

	testutil.BuildMember(t, db.DB)
	testutil.BuildMember(t, db.DB, testutil.WithMemberInactive())

	stats, err := repo.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Total < 2 {
		t.Errorf("Stats.Total: want ≥2, got %d", stats.Total)
	}
	if stats.Inactive < 1 {
		t.Errorf("Stats.Inactive: want ≥1, got %d", stats.Inactive)
	}
}

func TestMemberRepo_BirthdaysByMonth(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewMemberRepository(db)

	testutil.BuildMember(t, db.DB, testutil.WithBirthday(6, 15))
	testutil.BuildMember(t, db.DB, testutil.WithBirthday(6, 22))
	testutil.BuildMember(t, db.DB, testutil.WithBirthday(12, 1))

	members, err := repo.ListByMonth(6, true)
	if err != nil {
		t.Fatalf("ListByMonth: %v", err)
	}
	if len(members) < 2 {
		t.Errorf("expected ≥2 June birthday members, got %d", len(members))
	}
}
