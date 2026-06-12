-- Refresh tokens for JWT rotation (token family pattern).
-- Access tokens become short-lived (15m); refresh tokens long-lived (7d).
-- A replay of any revoked token in a family revokes the entire family.

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

-- Fast lookup by family for replay-attack revocation
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_family
    ON refresh_tokens(family_id);

-- Fast lookup by user for global logout
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user
    ON refresh_tokens(user_id);

-- Background cleanup of expired, non-revoked tokens
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_cleanup
    ON refresh_tokens(expires_at)
    WHERE revoked_at IS NULL;
