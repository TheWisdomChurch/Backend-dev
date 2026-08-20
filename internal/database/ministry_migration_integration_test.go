package database

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

// normalizeMinistryWorkforceMigrationSQL is the ministry/workforce normalization
// migration, originally 009_normalize_ministry_workforce.up.sql and now folded into
// the 011 logical section in migrations/schema.up.sql. It's kept here verbatim (not
// read from that file) because the file also contains sections that depend on tables
// (users, refresh_tokens, etc.) this test's minimal legacy-schema fixture doesn't have.
const normalizeMinistryWorkforceMigrationSQL = `
-- Fail before changing data when the deployed legacy schema does not satisfy
-- this migration's contract. This produces one actionable error instead of a
-- sequence of column failures during production deployment.
DO $$
DECLARE
    missing_columns text;
BEGIN
    SELECT string_agg(required.table_name || '.' || required.column_name, ', ' ORDER BY required.table_name, required.column_name)
    INTO missing_columns
    FROM (VALUES
        ('members', 'id'), ('members', 'email'),
        ('ministries', 'id'), ('ministries', 'name'), ('ministries', 'leader_id'),
        ('ministries', 'deleted_at'), ('ministries', 'created_at'),
        ('ministry_members', 'ministry_id'), ('ministry_members', 'member_id'),
        ('ministry_members', 'role'), ('ministry_members', 'joined_at'), ('ministry_members', 'deleted_at'),
        ('workforce_members', 'id'), ('workforce_members', 'email'),
        ('workforce_members', 'department'), ('workforce_members', 'created_at')
    ) AS required(table_name, column_name)
    LEFT JOIN information_schema.columns actual
      ON actual.table_schema = current_schema()
     AND actual.table_name = required.table_name
     AND actual.column_name = required.column_name
    WHERE actual.column_name IS NULL;

    IF missing_columns IS NOT NULL THEN
        RAISE EXCEPTION 'migration 009 schema contract failed; missing columns: %', missing_columns;
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS ministry_workforce_members (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    ministry_id uuid NOT NULL REFERENCES ministries(id) ON DELETE CASCADE,
    workforce_member_id uuid NOT NULL REFERENCES workforce_members(id) ON DELETE CASCADE,
    role varchar(30) NOT NULL DEFAULT 'member',
    title varchar(120),
    source varchar(30) NOT NULL DEFAULT 'manual',
    joined_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    CONSTRAINT ministry_workforce_role_check CHECK (role IN ('head','deputy_head','coordinator','member'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ministry_workforce_active_unique
    ON ministry_workforce_members(ministry_id, workforce_member_id)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_ministry_workforce_ministry_role
    ON ministry_workforce_members(ministry_id, role)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_ministry_workforce_member
    ON ministry_workforce_members(workforce_member_id)
    WHERE deleted_at IS NULL;

-- Materialize ministries for real workforce departments that do not already
-- have a case-insensitive ministry record. The workforce record remains the
-- authoritative person source; this only establishes organization structure.
WITH normalized_departments AS (
    SELECT
        lower(trim(w.department)) AS normalized_name,
        min(trim(w.department)) AS display_name
    FROM workforce_members w
    WHERE trim(COALESCE(w.department, '')) <> ''
    GROUP BY lower(trim(w.department))
)
INSERT INTO ministries (id, name, description, category, is_active, created_at, updated_at)
SELECT gen_random_uuid(), d.display_name, 'Created from existing workforce department assignments.', 'department', true, now(), now()
FROM normalized_departments d
WHERE NOT EXISTS (
    SELECT 1 FROM ministries m
    WHERE m.deleted_at IS NULL AND lower(trim(m.name)) = d.normalized_name
);

-- Backfill every workforce record into its matching ministry. Re-running is safe.
-- If legacy data contains duplicate active ministry names, use exactly one
-- canonical record (oldest, then UUID) rather than multiplying assignments.
WITH canonical_ministries AS (
    SELECT id, normalized_name
    FROM (
        SELECT
            m.id,
            lower(trim(m.name)) AS normalized_name,
            row_number() OVER (
                PARTITION BY lower(trim(m.name))
                ORDER BY m.created_at ASC NULLS LAST, m.id ASC
            ) AS position
        FROM ministries m
        WHERE m.deleted_at IS NULL
    ) ranked
    WHERE position = 1
)
INSERT INTO ministry_workforce_members (ministry_id, workforce_member_id, role, source, joined_at)
SELECT m.id, w.id, 'member', 'department_sync', COALESCE(w.created_at, now())
FROM workforce_members w
JOIN canonical_ministries m ON m.normalized_name = lower(trim(w.department))
WHERE trim(COALESCE(w.department, '')) <> ''
ON CONFLICT DO NOTHING;

-- Preserve the legacy ministries.leader_id only when it can be matched to an
-- actual workforce profile by an email that is unique in both source tables;
-- never infer by name or choose arbitrarily among duplicate email records.
WITH unique_member_emails AS (
    SELECT lower(trim(email)) AS normalized_email, min(id::text)::uuid AS member_id
    FROM members
    WHERE trim(COALESCE(email, '')) <> ''
    GROUP BY lower(trim(email))
    HAVING count(*) = 1
),
unique_workforce_emails AS (
    SELECT lower(trim(email)) AS normalized_email, min(id::text)::uuid AS workforce_member_id
    FROM workforce_members
    WHERE trim(COALESCE(email, '')) <> ''
    GROUP BY lower(trim(email))
    HAVING count(*) = 1
)
INSERT INTO ministry_workforce_members (ministry_id, workforce_member_id, role, source, joined_at)
SELECT m.id, workforce.workforce_member_id, 'head', 'legacy_leader', now()
FROM ministries m
JOIN unique_member_emails member_email ON member_email.member_id = m.leader_id
JOIN unique_workforce_emails workforce ON workforce.normalized_email = member_email.normalized_email
WHERE m.deleted_at IS NULL AND m.leader_id IS NOT NULL
ON CONFLICT (ministry_id, workforce_member_id) WHERE deleted_at IS NULL
DO UPDATE SET
    role = 'head',
    source = 'legacy_leader',
    updated_at = now();

-- Preserve legacy ministry membership where the member and workforce records
-- can be deterministically matched by a normalized email that is unique in
-- both tables. Collapse duplicate legacy membership rows and retain the
-- highest role before upserting, so an existing head is never downgraded.
WITH unique_member_emails AS (
    SELECT lower(trim(email)) AS normalized_email, min(id::text)::uuid AS member_id
    FROM members
    WHERE trim(COALESCE(email, '')) <> ''
    GROUP BY lower(trim(email))
    HAVING count(*) = 1
),
unique_workforce_emails AS (
    SELECT lower(trim(email)) AS normalized_email, min(id::text)::uuid AS workforce_member_id
    FROM workforce_members
    WHERE trim(COALESCE(email, '')) <> ''
    GROUP BY lower(trim(email))
    HAVING count(*) = 1
),
legacy_assignments AS (
    SELECT
        mm.ministry_id,
        workforce.workforce_member_id,
        max(CASE WHEN lower(mm.role) IN ('head', 'leader') THEN 4
                 WHEN lower(mm.role) IN ('deputy', 'assistant', 'deputy_head') THEN 3
                 WHEN lower(mm.role) = 'coordinator' THEN 2
                 ELSE 1 END) AS role_priority,
        min(mm.joined_at) AS joined_at
    FROM ministry_members mm
    JOIN unique_member_emails member_email ON member_email.member_id = mm.member_id
    JOIN unique_workforce_emails workforce ON workforce.normalized_email = member_email.normalized_email
    WHERE mm.deleted_at IS NULL
    GROUP BY mm.ministry_id, workforce.workforce_member_id
)
INSERT INTO ministry_workforce_members (ministry_id, workforce_member_id, role, source, joined_at)
SELECT legacy.ministry_id, legacy.workforce_member_id,
       CASE WHEN legacy.role_priority = 4 THEN 'head'
            WHEN legacy.role_priority = 3 THEN 'deputy_head'
            WHEN legacy.role_priority = 2 THEN 'coordinator'
            ELSE 'member' END,
       'legacy_membership', legacy.joined_at
FROM legacy_assignments legacy
ON CONFLICT (ministry_id, workforce_member_id) WHERE deleted_at IS NULL
DO UPDATE SET
    role = CASE
        WHEN ministry_workforce_members.role = 'head' OR EXCLUDED.role = 'head' THEN 'head'
        WHEN ministry_workforce_members.role = 'deputy_head' OR EXCLUDED.role = 'deputy_head' THEN 'deputy_head'
        WHEN ministry_workforce_members.role = 'coordinator' OR EXCLUDED.role = 'coordinator' THEN 'coordinator'
        ELSE 'member'
    END,
    source = CASE
        WHEN ministry_workforce_members.role = 'head' THEN ministry_workforce_members.source
        ELSE EXCLUDED.source
    END,
    joined_at = LEAST(ministry_workforce_members.joined_at, EXCLUDED.joined_at),
    updated_at = now();
`

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

	execute := func() {
		t.Helper()
		if _, err := tx.Exec(normalizeMinistryWorkforceMigrationSQL); err != nil {
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
