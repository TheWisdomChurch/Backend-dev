-- Cell groups / small groups management.

CREATE TABLE IF NOT EXISTS cell_groups (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL,
    campus_id    UUID REFERENCES campuses(id) ON DELETE SET NULL,
    leader_id    UUID REFERENCES members(id) ON DELETE SET NULL,
    zone         TEXT NOT NULL DEFAULT '',
    max_capacity INT NOT NULL DEFAULT 0 CHECK (max_capacity >= 0),
    is_active    BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at   TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS cell_group_members (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id   UUID NOT NULL REFERENCES cell_groups(id) ON DELETE CASCADE,
    member_id  UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    role       TEXT NOT NULL DEFAULT 'member',
    joined_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT cell_group_members_unique UNIQUE (group_id, member_id)
);

CREATE TABLE IF NOT EXISTS cell_group_meetings (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id       UUID NOT NULL REFERENCES cell_groups(id) ON DELETE CASCADE,
    date           TIMESTAMPTZ NOT NULL,
    attendee_count INT NOT NULL DEFAULT 0 CHECK (attendee_count >= 0),
    notes          TEXT NOT NULL DEFAULT '',
    led_by_id      UUID REFERENCES members(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_cell_groups_campus    ON cell_groups(campus_id) WHERE campus_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_cell_group_members_m  ON cell_group_members(member_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_cell_group_meetings_g ON cell_group_meetings(group_id, date DESC) WHERE deleted_at IS NULL;
