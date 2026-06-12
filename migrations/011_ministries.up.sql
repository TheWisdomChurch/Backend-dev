-- Ministry / department management.

CREATE TABLE IF NOT EXISTS ministries (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    campus_id   UUID REFERENCES campuses(id) ON DELETE SET NULL,
    leader_id   UUID REFERENCES members(id) ON DELETE SET NULL,
    category    TEXT NOT NULL DEFAULT '',
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);

INSERT INTO ministries (name, category) VALUES
    ('Worship Team',        'worship'),
    ('Children Ministry',   'children'),
    ('Media & Technology',  'media'),
    ('Ushering',            'ushering'),
    ('Prayer Team',         'prayer'),
    ('Welfare',             'welfare')
ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS ministry_members (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ministry_id UUID NOT NULL REFERENCES ministries(id) ON DELETE CASCADE,
    member_id   UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    role        TEXT NOT NULL DEFAULT 'member',
    joined_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ,
    CONSTRAINT ministry_members_unique UNIQUE (ministry_id, member_id)
);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ministries_campus   ON ministries(campus_id) WHERE campus_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ministry_members_m  ON ministry_members(member_id) WHERE deleted_at IS NULL;
