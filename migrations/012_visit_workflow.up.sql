-- Durable plan-a-visit workflow.
--
-- This remains a separate migration rather than being folded into schema.up.sql:
-- existing environments have already recorded the baseline migration and would
-- never execute content added to that file. Every object here is safe to create
-- on both upgraded and fresh databases.

CREATE TABLE IF NOT EXISTS visit_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    first_name VARCHAR(120) NOT NULL,
    last_name VARCHAR(120) NOT NULL,
    email VARCHAR(255) NOT NULL,
    phone VARCHAR(60),
    service_date DATE NOT NULL,
    service_at TIMESTAMPTZ NOT NULL,
    service_type VARCHAR(120) NOT NULL,
    attendance INTEGER NOT NULL DEFAULT 1 CONSTRAINT chk_visit_requests_attendance CHECK (attendance BETWEEN 1 AND 20),
    notes TEXT,
    status VARCHAR(40) NOT NULL DEFAULT 'new' CONSTRAINT chk_visit_requests_status CHECK (status IN ('new','confirmed','contacted','arrived','no_show','completed','cancelled')),
    assigned_to UUID REFERENCES users(id) ON DELETE SET NULL,
    next_follow_up_at TIMESTAMPTZ,
    follow_up_notified_at TIMESTAMPTZ,
    last_contact_at TIMESTAMPTZ,
    confirmation_sent_at TIMESTAMPTZ,
    reminder_sent_at TIMESTAMPTZ,
    checked_in_at TIMESTAMPTZ,
    source_channel VARCHAR(120) NOT NULL DEFAULT 'frontend:web:plan-visit',
    idempotency_key VARCHAR(160) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Case-insensitive email lookup supports idempotency/debugging without forcing
-- callers to reproduce the stored casing.
CREATE INDEX IF NOT EXISTS idx_visit_requests_email_lower ON visit_requests(LOWER(email));
CREATE INDEX IF NOT EXISTS idx_visit_requests_service_at ON visit_requests(service_at);
CREATE INDEX IF NOT EXISTS idx_visit_requests_service_date ON visit_requests(service_date);
CREATE INDEX IF NOT EXISTS idx_visit_requests_service_type ON visit_requests(service_type);
CREATE INDEX IF NOT EXISTS idx_visit_requests_status ON visit_requests(status);
CREATE INDEX IF NOT EXISTS idx_visit_requests_assigned_to ON visit_requests(assigned_to) WHERE assigned_to IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_visit_requests_reminders_due
    ON visit_requests(service_at)
    WHERE reminder_sent_at IS NULL AND status NOT IN ('cancelled', 'completed');
CREATE INDEX IF NOT EXISTS idx_visit_requests_follow_ups_due
    ON visit_requests(next_follow_up_at)
    WHERE next_follow_up_at IS NOT NULL AND follow_up_notified_at IS NULL AND status NOT IN ('completed', 'cancelled');
