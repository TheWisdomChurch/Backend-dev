-- Attendance tracking.

CREATE TABLE IF NOT EXISTS service_types (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    campus_id  UUID REFERENCES campuses(id) ON DELETE SET NULL,
    is_active  BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

INSERT INTO service_types (name) VALUES
    ('Sunday First Service'),
    ('Sunday Second Service'),
    ('Wednesday Service'),
    ('Friday Vigil')
ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS attendance_sessions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campus_id       UUID REFERENCES campuses(id) ON DELETE SET NULL,
    service_type_id UUID NOT NULL REFERENCES service_types(id),
    date            DATE NOT NULL,
    head_count      INT NOT NULL DEFAULT 0 CHECK (head_count >= 0),
    notes           TEXT NOT NULL DEFAULT '',
    created_by_id   UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS attendance_records (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id     UUID NOT NULL REFERENCES attendance_sessions(id) ON DELETE CASCADE,
    member_id      UUID REFERENCES members(id) ON DELETE SET NULL,
    guest_name     TEXT NOT NULL DEFAULT '',
    checked_in_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    checked_in_via TEXT NOT NULL DEFAULT 'manual',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at     TIMESTAMPTZ,
    -- A member can only be checked in once per session.
    CONSTRAINT attendance_records_session_member_unique UNIQUE (session_id, member_id)
);

CREATE INDEX IF NOT EXISTS idx_attendance_sessions_date
    ON attendance_sessions(date DESC) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_attendance_sessions_campus_date
    ON attendance_sessions(campus_id, date DESC)
    WHERE campus_id IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_attendance_records_session
    ON attendance_records(session_id) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_attendance_records_member
    ON attendance_records(member_id, checked_in_at DESC)
    WHERE member_id IS NOT NULL AND deleted_at IS NULL;
