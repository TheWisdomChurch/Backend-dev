-- Giving / financial transactions.

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

-- Seed standard categories.
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

-- Indexes for common access patterns.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_giving_category_date
    ON giving_transactions(category_id, given_at DESC) WHERE deleted_at IS NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_giving_member
    ON giving_transactions(member_id, given_at DESC)
    WHERE member_id IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_giving_status
    ON giving_transactions(status, given_at DESC) WHERE deleted_at IS NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_giving_campus
    ON giving_transactions(campus_id, given_at DESC)
    WHERE campus_id IS NOT NULL AND deleted_at IS NULL;
