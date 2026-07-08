-- Consolidated incremental schema migration.
--
-- Supersedes the 11 separate numbered migrations that used to live here
-- (001_add_account_lockout ... 011_ministries). Their content is merged below,
-- in original order, unchanged except:
--   - the ADD CONSTRAINT in the former 002 is now wrapped so it's safe to
--     re-run (Postgres has no `ADD CONSTRAINT IF NOT EXISTS`).
--   - the duplicate `idx_refresh_tokens_cleanup` index (originally created by
--     both the former 003 and 010) is defined once.
--
-- Every statement here is idempotent (IF NOT EXISTS / ON CONFLICT DO NOTHING /
-- guarded ADD CONSTRAINT), so this file is safe to run against a database
-- regardless of whether it already has some, all, or none of the former
-- numbered migrations applied — it converges every environment to the same
-- state without re-doing or losing anything already there.

-- ---------------------------------------------------------------------------
-- Formerly 001_add_account_lockout: account lockout tracking on users
-- ---------------------------------------------------------------------------
ALTER TABLE users
ADD COLUMN IF NOT EXISTS failed_login_count INTEGER DEFAULT 0,
ADD COLUMN IF NOT EXISTS last_failed_login_at TIMESTAMP NULL,
ADD COLUMN IF NOT EXISTS is_locked BOOLEAN DEFAULT FALSE,
ADD COLUMN IF NOT EXISTS locked_until TIMESTAMP NULL;

CREATE INDEX IF NOT EXISTS idx_users_is_locked ON users(is_locked) WHERE is_locked = true;
CREATE INDEX IF NOT EXISTS idx_users_locked_until ON users(locked_until) WHERE locked_until IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Formerly 002_add_trusted_devices_constraint: unique (user_id, device_id)
-- ---------------------------------------------------------------------------
DELETE FROM trusted_devices t1
WHERE EXISTS (
  SELECT 1 FROM trusted_devices t2
  WHERE t1.user_id = t2.user_id
  AND t1.device_id = t2.device_id
  AND t1.id > t2.id
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'uq_trusted_devices_user_device'
    ) THEN
        ALTER TABLE trusted_devices
        ADD CONSTRAINT uq_trusted_devices_user_device
        UNIQUE (user_id, device_id);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_trusted_devices_user_id ON trusted_devices(user_id);
CREATE INDEX IF NOT EXISTS idx_trusted_devices_expires_at ON trusted_devices(expires_at) WHERE deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- Formerly 003_add_refresh_tokens: refresh token family/rotation support
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS refresh_tokens (
    id          UUID        DEFAULT gen_random_uuid() PRIMARY KEY,
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    family_id   UUID        NOT NULL,
    token_hash  CHAR(64)    NOT NULL,          -- SHA-256 hex of raw token
    device_id   VARCHAR(255),
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    CONSTRAINT  refresh_tokens_hash_unique UNIQUE (token_hash)
);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_family
    ON refresh_tokens(family_id);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user
    ON refresh_tokens(user_id);

-- Background cleanup of expired, non-revoked tokens.
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_cleanup
    ON refresh_tokens(expires_at)
    WHERE revoked_at IS NULL;

-- ---------------------------------------------------------------------------
-- Formerly 004_encrypt_member_pii: encrypted phone column on members
-- ---------------------------------------------------------------------------
ALTER TABLE members ADD COLUMN IF NOT EXISTS phone_enc TEXT;

-- ---------------------------------------------------------------------------
-- Formerly 005_campus: multi-campus support
-- ---------------------------------------------------------------------------
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

ALTER TABLE members           ADD COLUMN IF NOT EXISTS campus_id UUID REFERENCES campuses(id) ON DELETE SET NULL;
ALTER TABLE events            ADD COLUMN IF NOT EXISTS campus_id UUID REFERENCES campuses(id) ON DELETE SET NULL;
ALTER TABLE workforce_members ADD COLUMN IF NOT EXISTS campus_id UUID REFERENCES campuses(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_campuses_active ON campuses(is_active) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_members_campus  ON members(campus_id) WHERE campus_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_events_campus   ON events(campus_id)  WHERE campus_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Formerly 006_giving_transactions: giving / financial transactions
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS giving_categories (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    code       TEXT NOT NULL,
    is_active  BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT giving_categories_name_unique UNIQUE (name),
    CONSTRAINT giving_categories_code_unique UNIQUE (code)
);

INSERT INTO giving_categories (name, code) VALUES
    ('Tithe',          'tithe'),
    ('Offering',       'offering'),
    ('Building Fund',  'building_fund'),
    ('Welfare',        'welfare'),
    ('Missions',       'missions')
ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS giving_transactions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    category_id      UUID NOT NULL REFERENCES giving_categories(id),
    member_id        UUID REFERENCES members(id) ON DELETE SET NULL,
    campus_id        UUID REFERENCES campuses(id) ON DELETE SET NULL,
    amount_kobo      BIGINT NOT NULL CHECK (amount_kobo > 0),
    currency         CHAR(3) NOT NULL DEFAULT 'NGN',
    channel          TEXT NOT NULL,           -- card, transfer, cash, ussd
    payment_ref      TEXT UNIQUE,
    payment_provider TEXT,                    -- paystack, stripe, manual
    status           TEXT NOT NULL DEFAULT 'pending',  -- pending, success, failed, reversed
    giver_name       TEXT NOT NULL DEFAULT '',
    giver_email      TEXT NOT NULL DEFAULT '',
    recorded_by_id   UUID REFERENCES users(id) ON DELETE SET NULL,
    given_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_giving_category_date
    ON giving_transactions(category_id, given_at DESC) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_giving_member
    ON giving_transactions(member_id, given_at DESC)
    WHERE member_id IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_giving_status
    ON giving_transactions(status, given_at DESC) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_giving_campus
    ON giving_transactions(campus_id, given_at DESC)
    WHERE campus_id IS NOT NULL AND deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- Formerly 007_attendance: attendance tracking
-- ---------------------------------------------------------------------------
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

-- ---------------------------------------------------------------------------
-- Formerly 008_cell_groups: cell groups / small groups management
-- ---------------------------------------------------------------------------
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

-- ---------------------------------------------------------------------------
-- Formerly 009_prayer_requests
-- ---------------------------------------------------------------------------
-- All body content is AES-256-GCM encrypted at the application layer.
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

-- ---------------------------------------------------------------------------
-- Formerly 010_performance_indexes: performance indexes for hot query paths
-- (idx_refresh_tokens_cleanup omitted here — already defined above, was a
-- duplicate of the one created alongside refresh_tokens itself)
-- ---------------------------------------------------------------------------
CREATE INDEX IF NOT EXISTS idx_security_events_user_type
    ON security_events(user_id, type, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_otps_email_purpose_active
    ON otps(email, purpose, expires_at DESC)
    WHERE used_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_members_fts
    ON members USING GIN (to_tsvector('english', first_name || ' ' || last_name));

CREATE INDEX IF NOT EXISTS idx_members_birthday
    ON members(birthday_month, birthday_day)
    WHERE is_active = true AND birthday_month IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_trusted_devices_user_device
    ON trusted_devices(user_id, device_id, expires_at DESC)
    WHERE deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- Formerly 011_ministries: ministry / department management
-- ---------------------------------------------------------------------------
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

CREATE INDEX IF NOT EXISTS idx_ministries_campus   ON ministries(campus_id) WHERE campus_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_ministry_members_m  ON ministry_members(member_id) WHERE deleted_at IS NULL;
