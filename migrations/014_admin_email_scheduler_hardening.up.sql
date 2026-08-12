ALTER TABLE admin_email_schedules
  ADD COLUMN IF NOT EXISTS start_date date,
  ADD COLUMN IF NOT EXISTS end_date date,
  ADD COLUMN IF NOT EXISTS pending_occurrence_at timestamptz,
  ADD COLUMN IF NOT EXISTS version integer NOT NULL DEFAULT 1;

UPDATE admin_email_schedules
SET start_date = (start_at AT TIME ZONE timezone)::date
WHERE start_date IS NULL;

UPDATE admin_email_schedules
SET end_date = (end_at AT TIME ZONE timezone)::date
WHERE end_at IS NOT NULL AND end_date IS NULL;

ALTER TABLE admin_email_schedules ALTER COLUMN start_date SET NOT NULL;

ALTER TABLE admin_email_schedule_runs
  ADD COLUMN IF NOT EXISTS attempt integer NOT NULL DEFAULT 1;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'admin_email_schedule_runs_status') THEN
    ALTER TABLE admin_email_schedule_runs ADD CONSTRAINT admin_email_schedule_runs_status
      CHECK (status IN ('running', 'completed', 'partial', 'failed'));
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_admin_email_schedules_pending_occurrence
  ON admin_email_schedules(pending_occurrence_at)
  WHERE pending_occurrence_at IS NOT NULL AND deleted_at IS NULL;
