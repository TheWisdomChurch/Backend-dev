package database

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

func TestNormalizeMinistryWorkforceMigration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}

	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer tx.Rollback()

	schema := "migration_009_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := tx.Exec(`CREATE SCHEMA ` + schema + `; SET LOCAL search_path TO ` + schema); err != nil {
		t.Fatalf("create isolated schema: %v", err)
	}

	legacySchema := `
		CREATE TABLE members (
			id uuid PRIMARY KEY, email text NOT NULL
		);
		CREATE TABLE workforce_members (
			id uuid PRIMARY KEY, email text, department text NOT NULL, created_at timestamptz NOT NULL
		);
		CREATE TABLE ministries (
			id uuid PRIMARY KEY, name text NOT NULL, description text NOT NULL DEFAULT '',
			category text NOT NULL DEFAULT '', is_active boolean NOT NULL DEFAULT true,
			leader_id uuid REFERENCES members(id), created_at timestamptz NOT NULL,
			updated_at timestamptz NOT NULL, deleted_at timestamptz
		);
		CREATE TABLE ministry_members (
			id uuid PRIMARY KEY, ministry_id uuid NOT NULL REFERENCES ministries(id),
			member_id uuid NOT NULL REFERENCES members(id), role text NOT NULL,
			joined_at timestamptz NOT NULL, deleted_at timestamptz
		);
	`
	if _, err := tx.Exec(legacySchema); err != nil {
		t.Fatalf("create production-like legacy schema: %v", err)
	}

	seed := `
		INSERT INTO members (id, email) VALUES
		('00000000-0000-0000-0000-000000000001', 'head@example.com'),
		('00000000-0000-0000-0000-000000000002', 'duplicate@example.com'),
		('00000000-0000-0000-0000-000000000003', 'DUPLICATE@example.com');

		INSERT INTO workforce_members (id, email, department, created_at) VALUES
		('10000000-0000-0000-0000-000000000001', 'HEAD@example.com', 'Choir', '2024-01-01'),
		('10000000-0000-0000-0000-000000000002', 'duplicate@example.com', 'choir', '2024-01-02'),
		('10000000-0000-0000-0000-000000000003', 'DUPLICATE@example.com', 'CHOIR', '2024-01-03'),
		('10000000-0000-0000-0000-000000000004', 'media@example.com', 'Media', '2024-01-04');

		INSERT INTO ministries (id, name, leader_id, created_at, updated_at) VALUES
		('20000000-0000-0000-0000-000000000001', 'Choir', '00000000-0000-0000-0000-000000000001', '2023-01-01', '2023-01-01'),
		('20000000-0000-0000-0000-000000000002', ' choir ', NULL, '2023-02-01', '2023-02-01');

		INSERT INTO ministry_members (id, ministry_id, member_id, role, joined_at) VALUES
		('30000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001', 'coordinator', '2023-03-01'),
		('30000000-0000-0000-0000-000000000002', '20000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000002', 'head', '2023-03-02');
	`
	if _, err := tx.Exec(seed); err != nil {
		t.Fatalf("seed edge-case data: %v", err)
	}

	migrationPath := filepath.Join("..", "..", "migrations", "009_normalize_ministry_workforce.up.sql")
	migration, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}

	execute := func() {
		t.Helper()
		if _, err := tx.Exec(string(migration)); err != nil {
			t.Fatalf("execute migration: %v", err)
		}
	}
	execute()

	assertCount(t, tx, 1, `SELECT count(*) FROM ministries WHERE lower(trim(name)) = 'media' AND deleted_at IS NULL`)
	assertCount(t, tx, 1, `
		SELECT count(*) FROM ministry_workforce_members
		WHERE workforce_member_id = '10000000-0000-0000-0000-000000000004' AND deleted_at IS NULL
	`)
	assertCount(t, tx, 1, `
		SELECT count(*) FROM ministry_workforce_members
		WHERE workforce_member_id = '10000000-0000-0000-0000-000000000001'
		  AND ministry_id = '20000000-0000-0000-0000-000000000001'
		  AND role = 'head' AND deleted_at IS NULL
	`)
	assertCount(t, tx, 0, `
		SELECT count(*) FROM ministry_workforce_members
		WHERE workforce_member_id IN (
			'10000000-0000-0000-0000-000000000002',
			'10000000-0000-0000-0000-000000000003'
		) AND role = 'head' AND deleted_at IS NULL
	`)

	var assignmentsBefore int
	if err := tx.QueryRow(`SELECT count(*) FROM ministry_workforce_members WHERE deleted_at IS NULL`).Scan(&assignmentsBefore); err != nil {
		t.Fatalf("count assignments before rerun: %v", err)
	}
	execute()
	assertCount(t, tx, assignmentsBefore, `SELECT count(*) FROM ministry_workforce_members WHERE deleted_at IS NULL`)
}

func assertCount(t *testing.T, tx *sql.Tx, expected int, query string) {
	t.Helper()
	var actual int
	if err := tx.QueryRow(query).Scan(&actual); err != nil {
		t.Fatalf("query count: %v\nquery: %s", err, query)
	}
	if actual != expected {
		t.Fatalf("unexpected count: got %d, want %d\nquery: %s", actual, expected, query)
	}
}
