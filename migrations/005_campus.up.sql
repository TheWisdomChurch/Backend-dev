-- Multi-campus support.

CREATE TABLE IF NOT EXISTS campuses (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    address     TEXT NOT NULL DEFAULT '',
    city        TEXT NOT NULL DEFAULT '',
    phone_enc   TEXT,
    time_zone   TEXT NOT NULL DEFAULT 'UTC',
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ,
    CONSTRAINT campuses_name_unique UNIQUE (name)
);

-- Nullable campus FK on members, events, and workforce_members — nullable so existing rows are unaffected.
ALTER TABLE members           ADD COLUMN IF NOT EXISTS campus_id UUID REFERENCES campuses(id) ON DELETE SET NULL;
ALTER TABLE events            ADD COLUMN IF NOT EXISTS campus_id UUID REFERENCES campuses(id) ON DELETE SET NULL;
ALTER TABLE workforce_members ADD COLUMN IF NOT EXISTS campus_id UUID REFERENCES campuses(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_campuses_active ON campuses(is_active) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_members_campus  ON members(campus_id) WHERE campus_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_events_campus   ON events(campus_id)  WHERE campus_id IS NOT NULL;
