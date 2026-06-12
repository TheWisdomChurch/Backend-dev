-- Prayer requests — all body content is AES-256-GCM encrypted at the application layer.

CREATE TABLE IF NOT EXISTS prayer_requests (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    member_id    UUID REFERENCES members(id) ON DELETE SET NULL,
    first_name   TEXT NOT NULL DEFAULT '',
    last_name    TEXT NOT NULL DEFAULT '',
    email        TEXT NOT NULL DEFAULT '',
    request_enc  TEXT NOT NULL,        -- AES-256-GCM ciphertext, never plaintext
    category     TEXT NOT NULL DEFAULT '',
    is_anonymous BOOLEAN NOT NULL DEFAULT FALSE,
    status       TEXT NOT NULL DEFAULT 'pending',  -- pending, praying, answered, closed
    assigned_to  UUID REFERENCES users(id) ON DELETE SET NULL,
    notes_enc    TEXT,                 -- AES-256-GCM encrypted pastoral notes
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_prayer_requests_status
    ON prayer_requests(status, created_at DESC) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_prayer_requests_member
    ON prayer_requests(member_id) WHERE member_id IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_prayer_requests_assigned
    ON prayer_requests(assigned_to) WHERE assigned_to IS NOT NULL AND deleted_at IS NULL;
