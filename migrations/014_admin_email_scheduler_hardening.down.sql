ALTER TABLE admin_email_schedule_runs DROP CONSTRAINT IF EXISTS admin_email_schedule_runs_status;
ALTER TABLE admin_email_schedule_runs DROP COLUMN IF EXISTS attempt;
DROP INDEX IF EXISTS idx_admin_email_schedules_pending_occurrence;
ALTER TABLE admin_email_schedules
  DROP COLUMN IF EXISTS version,
  DROP COLUMN IF EXISTS pending_occurrence_at,
  DROP COLUMN IF EXISTS end_date,
  DROP COLUMN IF EXISTS start_date;
