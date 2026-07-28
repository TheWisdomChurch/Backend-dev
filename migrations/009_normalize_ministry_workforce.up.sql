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
INSERT INTO ministries (id, name, description, category, is_active, created_at, updated_at)
SELECT gen_random_uuid(), trim(w.department), 'Created from existing workforce department assignments.', 'department', true, now(), now()
FROM workforce_members w
WHERE trim(COALESCE(w.department, '')) <> ''
GROUP BY trim(w.department)
HAVING NOT EXISTS (
    SELECT 1 FROM ministries m
    WHERE m.deleted_at IS NULL AND lower(trim(m.name)) = lower(trim(w.department))
);

-- Backfill every workforce record into its matching ministry. Re-running is safe.
INSERT INTO ministry_workforce_members (ministry_id, workforce_member_id, role, source, joined_at)
SELECT m.id, w.id, 'member', 'department_sync', COALESCE(w.created_at, now())
FROM workforce_members w
JOIN ministries m ON m.deleted_at IS NULL AND lower(trim(m.name)) = lower(trim(w.department))
WHERE trim(COALESCE(w.department, '')) <> ''
ON CONFLICT DO NOTHING;

-- Preserve the legacy ministries.leader_id only when it can be matched to an
-- actual workforce profile by verified email; never infer by name.
INSERT INTO ministry_workforce_members (ministry_id, workforce_member_id, role, source, joined_at)
SELECT m.id, w.id, 'head', 'manual', now()
FROM ministries m
JOIN members mem ON mem.id = m.leader_id AND mem.deleted_at IS NULL
JOIN workforce_members w ON lower(trim(w.email)) = lower(trim(mem.email))
WHERE m.deleted_at IS NULL AND m.leader_id IS NOT NULL AND trim(COALESCE(mem.email, '')) <> ''
ON CONFLICT DO NOTHING;

-- Preserve legacy ministry membership where the member and workforce records
-- can be deterministically matched by normalized email. Ambiguous/no-email
-- records are intentionally not guessed.
INSERT INTO ministry_workforce_members (ministry_id, workforce_member_id, role, source, joined_at)
SELECT mm.ministry_id, w.id,
       CASE WHEN lower(mm.role) IN ('head','leader') THEN 'head'
            WHEN lower(mm.role) IN ('deputy','assistant') THEN 'deputy_head'
            WHEN lower(mm.role) = 'coordinator' THEN 'coordinator'
            ELSE 'member' END,
       'manual', mm.joined_at
FROM ministry_members mm
JOIN members mem ON mem.id = mm.member_id AND mem.deleted_at IS NULL
JOIN workforce_members w ON lower(trim(w.email)) = lower(trim(mem.email))
WHERE mm.deleted_at IS NULL AND trim(COALESCE(mem.email, '')) <> ''
ON CONFLICT DO NOTHING;
