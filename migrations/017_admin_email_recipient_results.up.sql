ALTER TABLE admin_email_deliveries
  ADD COLUMN IF NOT EXISTS recipient_results jsonb NOT NULL DEFAULT '[]'::jsonb;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_admin_email_recipient_results_array') THEN
    ALTER TABLE admin_email_deliveries
      ADD CONSTRAINT chk_admin_email_recipient_results_array
      CHECK (jsonb_typeof(recipient_results) = 'array') NOT VALID;
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_admin_email_delivery_recipient_results
  ON admin_email_deliveries USING gin (recipient_results);
