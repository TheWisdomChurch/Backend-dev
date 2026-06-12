-- Performance indexes — all created CONCURRENTLY for zero-downtime deployment.
-- Run this migration outside a transaction (CONCURRENTLY is not allowed inside one).

-- Security events: most common query is "events for user X, by type, newest first"
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_security_events_user_type
    ON security_events(user_id, type, created_at DESC)
    WHERE deleted_at IS NULL;

-- OTPs: lookup by email + purpose for unexpired, unused codes
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_otps_email_purpose_active
    ON otps(email, purpose, expires_at DESC)
    WHERE used_at IS NULL;

-- Members: full-text search on first + last name
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_members_fts
    ON members USING GIN (to_tsvector('english', first_name || ' ' || last_name));

-- Members: birthday automation query (birthday_month + birthday_day + is_active)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_members_birthday
    ON members(birthday_month, birthday_day)
    WHERE is_active = true AND birthday_month IS NOT NULL;

-- Refresh tokens: cleanup job (delete expired, non-revoked)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_refresh_tokens_cleanup
    ON refresh_tokens(expires_at)
    WHERE revoked_at IS NULL;

-- Trusted devices: lookup by user + device
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_trusted_devices_user_device
    ON trusted_devices(user_id, device_id, expires_at DESC)
    WHERE deleted_at IS NULL;
