ALTER TABLE form_calendar_reminders ADD COLUMN IF NOT EXISTS delivery_status varchar(24) NOT NULL DEFAULT 'pending';
ALTER TABLE form_calendar_reminders ADD COLUMN IF NOT EXISTS delivery_attempt int NOT NULL DEFAULT 0;
ALTER TABLE form_calendar_reminders ADD COLUMN IF NOT EXISTS claimed_at timestamptz;
ALTER TABLE form_calendar_reminders ADD COLUMN IF NOT EXISTS claimed_by varchar(120);
ALTER TABLE form_calendar_reminders ADD COLUMN IF NOT EXISTS last_error text;

UPDATE form_calendar_reminders SET delivery_status = 'provider_accepted' WHERE reminder_sent_at IS NOT NULL;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_form_reminder_delivery_status') THEN
    ALTER TABLE form_calendar_reminders ADD CONSTRAINT chk_form_reminder_delivery_status
      CHECK (delivery_status IN ('pending','processing','failed','provider_accepted')) NOT VALID;
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_form_reminders_due_delivery
  ON form_calendar_reminders(event_starts_at, delivery_status, claimed_at)
  WHERE opted_in_at IS NOT NULL AND reminder_sent_at IS NULL;
