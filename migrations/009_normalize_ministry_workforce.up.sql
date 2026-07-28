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
