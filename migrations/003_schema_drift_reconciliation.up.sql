-- Reconciles tables/columns that exist only in Go models + conditional GORM
-- AutoMigrate (gated behind RUN_AUTOMIGRATE / ENSURE_ADMIN_SCHEMA_ON_STARTUP in
-- production, see internal/database/postgre.go) but were never captured in the
-- version-controlled raw SQL schema. Any environment provisioned from
-- migrations/*.sql alone (fresh deploy, staging, disaster recovery) was missing
-- these entirely. All statements are idempotent and additive only.

-- =========================
-- EVENTS: approval workflow columns (internal/models/event.go)
-- =========================
ALTER TABLE events ADD COLUMN IF NOT EXISTS is_approved boolean NOT NULL DEFAULT false;
ALTER TABLE events ADD COLUMN IF NOT EXISTS approved_by_id uuid;
ALTER TABLE events ADD COLUMN IF NOT EXISTS approved_by_name varchar(120);
ALTER TABLE events ADD COLUMN IF NOT EXISTS approved_by_email varchar(255);
ALTER TABLE events ADD COLUMN IF NOT EXISTS approved_at timestamptz;

-- =========================
-- TESTIMONIALS: approval attribution columns (internal/models/testimonials.go)
-- =========================
ALTER TABLE testimonials ADD COLUMN IF NOT EXISTS approved_by_id uuid;
ALTER TABLE testimonials ADD COLUMN IF NOT EXISTS approved_by_name varchar(120);
ALTER TABLE testimonials ADD COLUMN IF NOT EXISTS approved_by_email varchar(255);
ALTER TABLE testimonials ADD COLUMN IF NOT EXISTS approved_at timestamptz;

-- =========================
-- ADMIN_NOTIFICATIONS (internal/models/admin_notification.go)
-- =========================
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

-- =========================
-- APPROVAL_REQUESTS (internal/models/approval_request.go)
-- =========================
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

-- =========================
-- FORM_CAMPAIGN_DELIVERIES (internal/models/form_campaign_delivery.go)
-- =========================
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

-- =========================
-- ADMIN_EMAIL_DELIVERIES (internal/models/admin_email_delivery.go)
-- =========================
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

-- =========================
-- REGISTRATION_SEQUENCES (internal/models/registration_sequence.go)
-- =========================
CREATE TABLE IF NOT EXISTS registration_sequences (
    prefix      varchar(20) PRIMARY KEY,
    last_number int NOT NULL DEFAULT 0,
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- =========================
-- TICKET_SEQUENCES (internal/models/ticket_sequence.go)
-- =========================
CREATE TABLE IF NOT EXISTS ticket_sequences (
    prefix      varchar(40) PRIMARY KEY,
    last_number int NOT NULL DEFAULT 0,
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- =========================
-- ANALYTICS_BATCHES (internal/models/analytics_batch.go)
-- =========================
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

-- =========================
-- STORE_PRODUCTS / STORE_ORDERS / STORE_ORDER_ITEMS (internal/models/store.go)
-- =========================
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
