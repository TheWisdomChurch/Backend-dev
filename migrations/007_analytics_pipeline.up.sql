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
