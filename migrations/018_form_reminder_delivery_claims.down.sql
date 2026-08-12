DROP INDEX IF EXISTS idx_form_reminders_due_delivery;
ALTER TABLE form_calendar_reminders DROP CONSTRAINT IF EXISTS chk_form_reminder_delivery_status;
ALTER TABLE form_calendar_reminders DROP COLUMN IF EXISTS last_error;
ALTER TABLE form_calendar_reminders DROP COLUMN IF EXISTS claimed_by;
ALTER TABLE form_calendar_reminders DROP COLUMN IF EXISTS claimed_at;
ALTER TABLE form_calendar_reminders DROP COLUMN IF EXISTS delivery_attempt;
ALTER TABLE form_calendar_reminders DROP COLUMN IF EXISTS delivery_status;
