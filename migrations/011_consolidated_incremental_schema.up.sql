-- Consolidated incremental schema migration.
--
-- Supersedes the ten migration files that used to live here:
--   001_consolidated_incremental_schema (itself a prior consolidation of
--     account lockout, refresh tokens, campuses, giving, attendance, cell
--     groups, prayer requests, performance indexes, and ministries)
--   002_audit_logs
--   003_schema_drift_reconciliation
--   004_approval_request_reason
--   005_workforce_anniversary
--   006_prayer_request_integrity
--   007_analytics_pipeline
--   008_new_member_workflows
--   009_normalize_ministry_workforce
--   010_backfill_workforce_dates
-- Their content is merged below, unchanged, in the same order they were
-- originally applied (dependencies between them — e.g. 004 altering a table
-- 003 creates — require that order to be preserved).
--
-- Every statement here is idempotent (IF NOT EXISTS / ON CONFLICT DO NOTHING /
-- guarded ADD CONSTRAINT, or backfills keyed on natural idempotency such as
-- COALESCE-only UPDATEs), so this file is safe to run against a database
-- regardless of whether it already has none, some, or all of the ten former
-- files applied — it converges every environment to the same end state
-- without re-doing or losing anything already there.

-- =============================================================================
-- Formerly 001_consolidated_incremental_schema
-- =============================================================================

-- ---------------------------------------------------------------------------
-- Formerly 001/001_add_account_lockout: account lockout tracking on users
-- ---------------------------------------------------------------------------
ALTER TABLE users
ADD COLUMN IF NOT EXISTS failed_login_count INTEGER DEFAULT 0,
ADD COLUMN IF NOT EXISTS last_failed_login_at TIMESTAMP NULL,
ADD COLUMN IF NOT EXISTS is_locked BOOLEAN DEFAULT FALSE,
ADD COLUMN IF NOT EXISTS locked_until TIMESTAMP NULL;

CREATE INDEX IF NOT EXISTS idx_users_is_locked ON users(is_locked) WHERE is_locked = true;
CREATE INDEX IF NOT EXISTS idx_users_locked_until ON users(locked_until) WHERE locked_until IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Formerly 001/002_add_trusted_devices_constraint: unique (user_id, device_id)
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
-- Formerly 001/003_add_refresh_tokens: refresh token family/rotation support
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
-- Formerly 001/004_encrypt_member_pii: encrypted phone column on members
-- ---------------------------------------------------------------------------
ALTER TABLE members ADD COLUMN IF NOT EXISTS phone_enc TEXT;

-- ---------------------------------------------------------------------------
-- Formerly 001/005_campus: multi-campus support
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
-- Formerly 001/006_giving_transactions: giving / financial transactions
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
-- Formerly 001/007_attendance: attendance tracking
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
-- Formerly 001/008_cell_groups: cell groups / small groups management
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
-- Formerly 001/009_prayer_requests
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
-- Formerly 001/010_performance_indexes: performance indexes for hot query paths
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
-- Formerly 001/011_ministries: ministry / department management
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

-- =============================================================================
-- Formerly 002_audit_logs
--
-- Durable audit log storage. Previously, admin/auth mutating requests were
-- only ever written to structured application logs (internal/logger), never
-- to the database — so the "recent activity" dashboard widget and the
-- /admin/audit-logs endpoint had nothing real to show and returned hardcoded
-- empty results. This table gives them something to query.
-- =============================================================================
CREATE TABLE IF NOT EXISTS audit_logs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scope       TEXT NOT NULL,
    method      TEXT NOT NULL,
    path        TEXT NOT NULL,
    status_code INT NOT NULL,
    latency_ms  BIGINT NOT NULL,
    user_id     UUID REFERENCES users(id) ON DELETE SET NULL,
    role        TEXT NOT NULL DEFAULT '',
    ip          TEXT NOT NULL DEFAULT '',
    user_agent  TEXT NOT NULL DEFAULT '',
    request_id  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at
    ON audit_logs(created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_logs_scope_created_at
    ON audit_logs(scope, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id
    ON audit_logs(user_id) WHERE user_id IS NOT NULL;

-- =============================================================================
-- Formerly 003_schema_drift_reconciliation
--
-- Reconciles tables/columns that exist only in Go models + conditional GORM
-- AutoMigrate (gated behind RUN_AUTOMIGRATE / ENSURE_ADMIN_SCHEMA_ON_STARTUP in
-- production, see internal/database/postgre.go) but were never captured in the
-- version-controlled raw SQL schema. Any environment provisioned from
-- migrations/*.sql alone (fresh deploy, staging, disaster recovery) was missing
-- these entirely. All statements are idempotent and additive only.
-- =============================================================================

-- EVENTS: approval workflow columns (internal/models/event.go)
ALTER TABLE events ADD COLUMN IF NOT EXISTS is_approved boolean NOT NULL DEFAULT false;
ALTER TABLE events ADD COLUMN IF NOT EXISTS approved_by_id uuid;
ALTER TABLE events ADD COLUMN IF NOT EXISTS approved_by_name varchar(120);
ALTER TABLE events ADD COLUMN IF NOT EXISTS approved_by_email varchar(255);
ALTER TABLE events ADD COLUMN IF NOT EXISTS approved_at timestamptz;

-- TESTIMONIALS: approval attribution columns (internal/models/testimonials.go)
ALTER TABLE testimonials ADD COLUMN IF NOT EXISTS approved_by_id uuid;
ALTER TABLE testimonials ADD COLUMN IF NOT EXISTS approved_by_name varchar(120);
ALTER TABLE testimonials ADD COLUMN IF NOT EXISTS approved_by_email varchar(255);
ALTER TABLE testimonials ADD COLUMN IF NOT EXISTS approved_at timestamptz;

-- ADMIN_NOTIFICATIONS (internal/models/admin_notification.go)
CREATE TABLE IF NOT EXISTS admin_notifications (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL,
    type        varchar(40) NOT NULL,
    title       varchar(255) NOT NULL,
    message     text NOT NULL,
    ticket_code varchar(50),
    entity_type varchar(40),
    entity_id   uuid,
    is_read     boolean NOT NULL DEFAULT false,
    read_at     timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_admin_notifications_user_id ON admin_notifications(user_id);

-- APPROVAL_REQUESTS (internal/models/approval_request.go)
CREATE TABLE IF NOT EXISTS approval_requests (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_code        varchar(50) NOT NULL,
    type               varchar(30) NOT NULL,
    status             varchar(20) NOT NULL DEFAULT 'pending',
    entity_id          uuid,
    entity_label       varchar(255),
    requested_by_id    uuid,
    requested_by_name  varchar(120),
    requested_by_email varchar(255),
    approved_by_id     uuid,
    approved_by_name   varchar(120),
    approved_by_email  varchar(255),
    approved_at        timestamptz,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_approval_requests_ticket_code ON approval_requests(ticket_code);

-- FORM_CAMPAIGN_DELIVERIES (internal/models/form_campaign_delivery.go)
CREATE TABLE IF NOT EXISTS form_campaign_deliveries (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    form_id            uuid NOT NULL,
    form_title         varchar(255) NOT NULL,
    event_id           uuid,
    event_title        varchar(255),
    subject            varchar(255) NOT NULL,
    template_source    varchar(120) NOT NULL,
    template_id        varchar(120),
    template_key       varchar(255),
    status             varchar(20) NOT NULL DEFAULT 'completed',
    total_recipients   int NOT NULL DEFAULT 0,
    targeted           int NOT NULL DEFAULT 0,
    sent               int NOT NULL DEFAULT 0,
    skipped            int NOT NULL DEFAULT 0,
    failed             int NOT NULL DEFAULT 0,
    failed_recipients  jsonb,
    started_at         timestamptz NOT NULL,
    completed_at       timestamptz,
    created_by_user_id uuid,
    created_by_email   varchar(255),
    created_by_role    varchar(50),
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    deleted_at         timestamptz
);
CREATE INDEX IF NOT EXISTS idx_form_campaign_deliveries_form_id      ON form_campaign_deliveries(form_id);
CREATE INDEX IF NOT EXISTS idx_form_campaign_deliveries_event_id     ON form_campaign_deliveries(event_id);
CREATE INDEX IF NOT EXISTS idx_form_campaign_deliveries_status       ON form_campaign_deliveries(status);
CREATE INDEX IF NOT EXISTS idx_form_campaign_deliveries_started_at   ON form_campaign_deliveries(started_at);
CREATE INDEX IF NOT EXISTS idx_form_campaign_deliveries_completed_at ON form_campaign_deliveries(completed_at);
CREATE INDEX IF NOT EXISTS idx_form_campaign_deliveries_deleted_at   ON form_campaign_deliveries(deleted_at);

-- ADMIN_EMAIL_DELIVERIES (internal/models/admin_email_delivery.go)
CREATE TABLE IF NOT EXISTS admin_email_deliveries (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    subject            varchar(255) NOT NULL,
    template_source    varchar(120) NOT NULL,
    template_id        varchar(120),
    template_key       varchar(255),
    audience_source    varchar(20) NOT NULL DEFAULT 'manual',
    manual_recipients  int NOT NULL DEFAULT 0,
    form_recipients    int NOT NULL DEFAULT 0,
    source_forms       jsonb,
    status             varchar(20) NOT NULL DEFAULT 'completed',
    total_recipients   int NOT NULL DEFAULT 0,
    targeted           int NOT NULL DEFAULT 0,
    sent               int NOT NULL DEFAULT 0,
    skipped            int NOT NULL DEFAULT 0,
    failed             int NOT NULL DEFAULT 0,
    failed_recipients  jsonb,
    started_at         timestamptz NOT NULL,
    completed_at       timestamptz,
    created_by_user_id uuid,
    created_by_email   varchar(255),
    created_by_role    varchar(50),
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    deleted_at         timestamptz
);
CREATE INDEX IF NOT EXISTS idx_admin_email_deliveries_audience_source ON admin_email_deliveries(audience_source);
CREATE INDEX IF NOT EXISTS idx_admin_email_deliveries_status          ON admin_email_deliveries(status);
CREATE INDEX IF NOT EXISTS idx_admin_email_deliveries_started_at      ON admin_email_deliveries(started_at);
CREATE INDEX IF NOT EXISTS idx_admin_email_deliveries_completed_at    ON admin_email_deliveries(completed_at);
CREATE INDEX IF NOT EXISTS idx_admin_email_deliveries_deleted_at      ON admin_email_deliveries(deleted_at);

-- REGISTRATION_SEQUENCES (internal/models/registration_sequence.go)
CREATE TABLE IF NOT EXISTS registration_sequences (
    prefix      varchar(20) PRIMARY KEY,
    last_number int NOT NULL DEFAULT 0,
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- TICKET_SEQUENCES (internal/models/ticket_sequence.go)
CREATE TABLE IF NOT EXISTS ticket_sequences (
    prefix      varchar(40) PRIMARY KEY,
    last_number int NOT NULL DEFAULT 0,
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- ANALYTICS_BATCHES (internal/models/analytics_batch.go)
CREATE TABLE IF NOT EXISTS analytics_batches (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id    varchar(120) NOT NULL,
    session_id  varchar(120) NOT NULL,
    user_id     varchar(120),
    event_count int NOT NULL DEFAULT 0,
    payload     jsonb NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_analytics_batches_batch_id   ON analytics_batches(batch_id);
CREATE INDEX IF NOT EXISTS idx_analytics_batches_session_id ON analytics_batches(session_id);
CREATE INDEX IF NOT EXISTS idx_analytics_batches_user_id    ON analytics_batches(user_id);

-- STORE_PRODUCTS / STORE_ORDERS / STORE_ORDER_ITEMS (internal/models/store.go)
CREATE TABLE IF NOT EXISTS store_products (
    id             bigserial PRIMARY KEY,
    name           varchar(200) NOT NULL,
    category       varchar(80) NOT NULL,
    price          varchar(40) NOT NULL,
    original_price varchar(40),
    image          text NOT NULL,
    description    text NOT NULL,
    sizes          jsonb NOT NULL DEFAULT '[]',
    colors         jsonb NOT NULL DEFAULT '[]',
    tags           jsonb NOT NULL DEFAULT '[]',
    stock          int NOT NULL DEFAULT 0,
    is_active      boolean NOT NULL DEFAULT true,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_store_products_category ON store_products(category);

CREATE TABLE IF NOT EXISTS store_orders (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id              varchar(120) NOT NULL,
    status                varchar(30) NOT NULL DEFAULT 'pending',
    subtotal              double precision NOT NULL DEFAULT 0,
    delivery_fee          double precision NOT NULL DEFAULT 0,
    total                 double precision NOT NULL DEFAULT 0,
    payment_method        varchar(40) NOT NULL,
    customer_first_name   varchar(120) NOT NULL,
    customer_last_name    varchar(120) NOT NULL,
    customer_email        varchar(255) NOT NULL,
    customer_phone        varchar(64) NOT NULL,
    customer_address      text,
    customer_city         varchar(120),
    customer_state        varchar(120),
    customer_zip_code     varchar(40),
    customer_account_name varchar(180),
    customer_bank_name    varchar(180),
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_store_orders_order_id ON store_orders(order_id);
CREATE INDEX IF NOT EXISTS idx_store_orders_customer_email ON store_orders(customer_email);

CREATE TABLE IF NOT EXISTS store_order_items (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    store_order_id uuid NOT NULL,
    product_id     bigint,
    name           varchar(220) NOT NULL,
    price          varchar(40) NOT NULL,
    quantity       int NOT NULL DEFAULT 1,
    selected_size  varchar(80) NOT NULL,
    selected_color varchar(80) NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'fk_store_order_items_store_order'
    ) THEN
        ALTER TABLE store_order_items
            ADD CONSTRAINT fk_store_order_items_store_order
            FOREIGN KEY (store_order_id) REFERENCES store_orders(id) ON DELETE CASCADE;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_store_order_items_store_order_id ON store_order_items(store_order_id);
CREATE INDEX IF NOT EXISTS idx_store_order_items_product_id     ON store_order_items(product_id);

-- =============================================================================
-- Formerly 004_approval_request_reason
--
-- Adds the stated reason a requester gives for an approval request — needed
-- for delete-approval flows (event/workforce/leadership deletion) so the
-- super-admin reviewing the request has actual context instead of just an
-- entity label. Nullable: existing rows and non-delete request types don't
-- require one.
-- =============================================================================
ALTER TABLE approval_requests ADD COLUMN IF NOT EXISTS reason TEXT;

-- =============================================================================
-- Formerly 005_workforce_anniversary
--
-- Adds wedding-anniversary tracking to workforce_members, mirroring the
-- birthday month/day columns already there and the anniversary columns
-- leadership_members already has. Without this, a workforce registration
-- form asking for an anniversary date had nowhere to store the answer.
-- =============================================================================
ALTER TABLE workforce_members
  ADD COLUMN IF NOT EXISTS anniversary_month smallint CHECK (anniversary_month BETWEEN 1 AND 12),
  ADD COLUMN IF NOT EXISTS anniversary_day smallint CHECK (anniversary_day BETWEEN 1 AND 31);

-- =============================================================================
-- Formerly 006_prayer_request_integrity
-- =============================================================================
UPDATE prayer_requests
SET status = 'pending'
WHERE status NOT IN ('pending', 'praying', 'answered', 'closed');

ALTER TABLE prayer_requests
    DROP CONSTRAINT IF EXISTS chk_prayer_requests_status;

ALTER TABLE prayer_requests
    ADD CONSTRAINT chk_prayer_requests_status
    CHECK (status IN ('pending', 'praying', 'answered', 'closed'));

CREATE INDEX IF NOT EXISTS idx_prayer_requests_category_created
    ON prayer_requests(category, created_at DESC)
    WHERE deleted_at IS NULL;

-- =============================================================================
-- Formerly 007_analytics_pipeline
-- =============================================================================
ALTER TABLE events ADD COLUMN IF NOT EXISTS event_date date;

DO $$
DECLARE event_row record;
BEGIN
    FOR event_row IN
        SELECT id, date FROM events
        WHERE event_date IS NULL AND date ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$'
    LOOP
        BEGIN
            UPDATE events SET event_date = event_row.date::date WHERE id = event_row.id;
        EXCEPTION WHEN datetime_field_overflow OR invalid_datetime_format THEN
            RAISE WARNING 'Skipping invalid legacy event date for event %: %', event_row.id, event_row.date;
        END;
    END LOOP;
END $$;

CREATE INDEX IF NOT EXISTS idx_events_event_date ON events(event_date);
CREATE INDEX IF NOT EXISTS idx_events_category_event_date ON events(category, event_date);

CREATE OR REPLACE FUNCTION sync_event_native_date() RETURNS trigger AS $$
BEGIN
    NEW.event_date := NEW.date::date;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_events_sync_native_date ON events;
CREATE TRIGGER trg_events_sync_native_date
BEFORE INSERT OR UPDATE OF date ON events
FOR EACH ROW EXECUTE FUNCTION sync_event_native_date();

-- Make retries idempotent before normalized events reference the batch key.
DELETE FROM analytics_batches older
USING analytics_batches newer
WHERE older.batch_id = newer.batch_id
  AND (older.created_at, older.id) > (newer.created_at, newer.id);

DROP INDEX IF EXISTS idx_analytics_batches_batch_id;
CREATE UNIQUE INDEX IF NOT EXISTS idx_analytics_batches_batch_id_unique
    ON analytics_batches(batch_id);

ALTER TABLE analytics_batches
    ADD COLUMN IF NOT EXISTS expires_at timestamptz;
UPDATE analytics_batches SET expires_at = created_at + INTERVAL '30 days' WHERE expires_at IS NULL;
ALTER TABLE analytics_batches ALTER COLUMN expires_at SET DEFAULT (NOW() + INTERVAL '30 days');
ALTER TABLE analytics_batches ALTER COLUMN expires_at SET NOT NULL;
CREATE INDEX IF NOT EXISTS idx_analytics_batches_expires_at ON analytics_batches(expires_at);

CREATE TABLE IF NOT EXISTS analytics_events (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id        varchar(120) NOT NULL,
    session_id      varchar(120) NOT NULL,
    user_id         varchar(120),
    client_event_id varchar(120),
    category        varchar(80) NOT NULL,
    action          varchar(80) NOT NULL,
    occurred_at     timestamptz NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_analytics_event_category CHECK (category ~ '^[a-z0-9][a-z0-9._:-]{0,79}$'),
    CONSTRAINT chk_analytics_event_action CHECK (action ~ '^[a-z0-9][a-z0-9._:-]{0,79}$')
);

CREATE INDEX IF NOT EXISTS idx_analytics_events_occurred_at ON analytics_events(occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_analytics_events_category_time ON analytics_events(category, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_analytics_events_action_time ON analytics_events(action, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_analytics_events_session_time ON analytics_events(session_id, occurred_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_analytics_events_batch_client_id
    ON analytics_events(batch_id, client_event_id) WHERE client_event_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_form_submissions_form_created
    ON form_submissions(form_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_attendance_sessions_date_active
    ON attendance_sessions(date DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_attendance_records_session_active
    ON attendance_records(session_id) WHERE deleted_at IS NULL;

-- =============================================================================
-- Formerly 008_new_member_workflows
-- =============================================================================
CREATE TABLE IF NOT EXISTS new_member_workflows (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    submission_id uuid NOT NULL UNIQUE REFERENCES form_submissions(id) ON DELETE CASCADE,
    stage varchar(40) NOT NULL DEFAULT 'new',
    assigned_owner_id uuid REFERENCES users(id) ON DELETE SET NULL,
    next_action_at timestamptz,
    escalation_status varchar(30) NOT NULL DEFAULT 'none',
    escalated_at timestamptz,
    completed_at timestamptz,
    last_contacted_at timestamptz,
    last_reminder_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT new_member_workflow_stage_check CHECK (stage IN ('new','contact_attempted','contacted','orientation_scheduled','orientation_completed','integrated','closed')),
    CONSTRAINT new_member_workflow_escalation_check CHECK (escalation_status IN ('none','due','escalated','resolved'))
);

CREATE INDEX IF NOT EXISTS idx_new_member_workflows_owner ON new_member_workflows(assigned_owner_id, stage);
CREATE INDEX IF NOT EXISTS idx_new_member_workflows_due ON new_member_workflows(next_action_at) WHERE completed_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_new_member_workflows_escalation ON new_member_workflows(escalation_status, next_action_at);

CREATE TABLE IF NOT EXISTS new_member_contacts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id uuid NOT NULL REFERENCES new_member_workflows(id) ON DELETE CASCADE,
    channel varchar(30) NOT NULL,
    outcome varchar(50) NOT NULL,
    notes text,
    contacted_at timestamptz NOT NULL,
    created_by_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT new_member_contact_channel_check CHECK (channel IN ('phone','email','sms','whatsapp','in_person','other'))
);
CREATE INDEX IF NOT EXISTS idx_new_member_contacts_workflow ON new_member_contacts(workflow_id, contacted_at DESC);

CREATE TABLE IF NOT EXISTS new_member_workflow_history (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id uuid NOT NULL REFERENCES new_member_workflows(id) ON DELETE CASCADE,
    event_type varchar(50) NOT NULL,
    from_stage varchar(40),
    to_stage varchar(40),
    actor_id uuid REFERENCES users(id) ON DELETE SET NULL,
    details jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_new_member_history_workflow ON new_member_workflow_history(workflow_id, created_at DESC);

INSERT INTO new_member_workflows (submission_id, next_action_at)
SELECT fs.id, fs.created_at + INTERVAL '1 day'
FROM form_submissions fs
JOIN forms f ON f.id = fs.form_id
WHERE fs.deleted_at IS NULL AND f.deleted_at IS NULL
  AND (
    LOWER(COALESCE(f.settings->>'submissionTarget', '')) = 'member'
    OR LOWER(COALESCE(f.slug, '')) = 'add-new-member'
    OR trim(regexp_replace(LOWER(COALESCE(f.title, '')), '[^a-z0-9]+', ' ', 'g')) = 'add new member'
  )
ON CONFLICT (submission_id) DO NOTHING;

-- =============================================================================
-- Formerly 009_normalize_ministry_workforce
--
-- Fail before changing data when the deployed legacy schema does not satisfy
-- this migration's contract. This produces one actionable error instead of a
-- sequence of column failures during production deployment.
-- =============================================================================
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

-- =============================================================================
-- Formerly 010_backfill_workforce_dates
--
-- Ensure drifted production schemas have the recurring date columns even when
-- they were created before the current baseline/005 migration history.
-- =============================================================================
ALTER TABLE workforce_members
  ADD COLUMN IF NOT EXISTS birthday_month smallint,
  ADD COLUMN IF NOT EXISTS birthday_day smallint,
  ADD COLUMN IF NOT EXISTS anniversary_month smallint,
  ADD COLUMN IF NOT EXISTS anniversary_day smallint;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'workforce_birthday_month_check') THEN
    ALTER TABLE workforce_members ADD CONSTRAINT workforce_birthday_month_check CHECK (birthday_month BETWEEN 1 AND 12);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'workforce_birthday_day_check') THEN
    ALTER TABLE workforce_members ADD CONSTRAINT workforce_birthday_day_check CHECK (birthday_day BETWEEN 1 AND 31);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'workforce_anniversary_month_check') THEN
    ALTER TABLE workforce_members ADD CONSTRAINT workforce_anniversary_month_check CHECK (anniversary_month BETWEEN 1 AND 12);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'workforce_anniversary_day_check') THEN
    ALTER TABLE workforce_members ADD CONSTRAINT workforce_anniversary_day_check CHECK (anniversary_day BETWEEN 1 AND 31);
  END IF;
END $$;

-- Backfill only deterministic workforce submissions matched by normalized
-- email. Existing administrator-corrected values always win. Accepted stored
-- shapes are ISO YYYY-MM-DD and DD/MM[/YYYY] (also '-' or '.' separators).
WITH raw AS (
  SELECT fs.created_at, lower(trim(fs.email)) AS email,
    COALESCE(NULLIF(trim(fs.values->>'birthday'), ''), NULLIF(trim(fs.values->>'birthDate'), ''), NULLIF(trim(fs.values->>'birth_date'), ''), NULLIF(trim(fs.values->>'dob'), ''), NULLIF(trim(fs.values->>'dateOfBirth'), ''), NULLIF(trim(fs.values->>'date_of_birth'), '')) AS birthday,
    COALESCE(NULLIF(trim(fs.values->>'anniversary'), ''), NULLIF(trim(fs.values->>'weddingAnniversary'), ''), NULLIF(trim(fs.values->>'wedding_anniversary'), ''), NULLIF(trim(fs.values->>'anniversaryDate'), ''), NULLIF(trim(fs.values->>'anniversary_date'), '')) AS anniversary
  FROM form_submissions fs
  JOIN forms f ON f.id = fs.form_id AND f.deleted_at IS NULL
  WHERE fs.deleted_at IS NULL AND trim(COALESCE(fs.email, '')) <> ''
    AND (lower(COALESCE(f.settings->>'submissionTarget', '')) LIKE 'workforce%' OR lower(COALESCE(f.settings->>'formType', '')) = 'workforce' OR lower(COALESCE(f.slug, '')) LIKE '%workforce%')
), parsed AS (
  SELECT *,
    CASE WHEN birthday ~ '^\d{4}-\d{1,2}-\d{1,2}$' THEN split_part(birthday, '-', 2)::int WHEN birthday ~ '^\d{1,2}[/.-]\d{1,2}([/.-]\d{2,4})?$' THEN regexp_replace(birthday, '^\d{1,2}[/.-](\d{1,2}).*$', '\1')::int END AS bm,
    CASE WHEN birthday ~ '^\d{4}-\d{1,2}-\d{1,2}$' THEN split_part(birthday, '-', 3)::int WHEN birthday ~ '^\d{1,2}[/.-]\d{1,2}([/.-]\d{2,4})?$' THEN regexp_replace(birthday, '^(\d{1,2})[/.-].*$', '\1')::int END AS bd,
    CASE WHEN anniversary ~ '^\d{4}-\d{1,2}-\d{1,2}$' THEN split_part(anniversary, '-', 2)::int WHEN anniversary ~ '^\d{1,2}[/.-]\d{1,2}([/.-]\d{2,4})?$' THEN regexp_replace(anniversary, '^\d{1,2}[/.-](\d{1,2}).*$', '\1')::int END AS am,
    CASE WHEN anniversary ~ '^\d{4}-\d{1,2}-\d{1,2}$' THEN split_part(anniversary, '-', 3)::int WHEN anniversary ~ '^\d{1,2}[/.-]\d{1,2}([/.-]\d{2,4})?$' THEN regexp_replace(anniversary, '^(\d{1,2})[/.-].*$', '\1')::int END AS ad
  FROM raw
), valid AS (
  SELECT *,
    (bm BETWEEN 1 AND 12 AND bd BETWEEN 1 AND CASE bm WHEN 2 THEN 29 WHEN 4 THEN 30 WHEN 6 THEN 30 WHEN 9 THEN 30 WHEN 11 THEN 30 ELSE 31 END) AS birthday_valid,
    (am BETWEEN 1 AND 12 AND ad BETWEEN 1 AND CASE am WHEN 2 THEN 29 WHEN 4 THEN 30 WHEN 6 THEN 30 WHEN 9 THEN 30 WHEN 11 THEN 30 ELSE 31 END) AS anniversary_valid
  FROM parsed
), latest_birthday AS (
  SELECT DISTINCT ON (email) email, bm, bd FROM valid WHERE birthday_valid ORDER BY email, created_at DESC
), latest_anniversary AS (
  SELECT DISTINCT ON (email) email, am, ad FROM valid WHERE anniversary_valid ORDER BY email, created_at DESC
), dates AS (
  SELECT COALESCE(b.email, a.email) AS email, b.bm, b.bd, a.am, a.ad
  FROM latest_birthday b FULL OUTER JOIN latest_anniversary a ON a.email = b.email
)
UPDATE workforce_members w SET
  birthday_month = COALESCE(w.birthday_month, d.bm),
  birthday_day = COALESCE(w.birthday_day, d.bd),
  anniversary_month = COALESCE(w.anniversary_month, d.am),
  anniversary_day = COALESCE(w.anniversary_day, d.ad),
  updated_at = CASE WHEN (w.birthday_month IS NULL AND d.bm IS NOT NULL) OR (w.anniversary_month IS NULL AND d.am IS NOT NULL) THEN now() ELSE w.updated_at END
FROM dates d WHERE lower(trim(w.email)) = d.email;

CREATE INDEX IF NOT EXISTS idx_workforce_birthday_month_day ON workforce_members(birthday_month, birthday_day) WHERE birthday_month IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_workforce_anniversary_month_day ON workforce_members(anniversary_month, anniversary_day) WHERE anniversary_month IS NOT NULL;
