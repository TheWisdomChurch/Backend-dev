-- Durable audit log storage. Previously, admin/auth mutating requests were
-- only ever written to structured application logs (internal/logger), never
-- to the database — so the "recent activity" dashboard widget and the
-- /admin/audit-logs endpoint had nothing real to show and returned hardcoded
-- empty results. This table gives them something to query.

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
