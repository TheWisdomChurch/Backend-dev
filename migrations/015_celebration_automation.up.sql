CREATE TABLE IF NOT EXISTS celebration_automation_config (
  id varchar(40) PRIMARY KEY, enabled boolean NOT NULL DEFAULT false,
  birthday_enabled boolean NOT NULL DEFAULT true, anniversary_enabled boolean NOT NULL DEFAULT true,
  timezone varchar(80) NOT NULL, send_time char(5) NOT NULL, feb29_policy varchar(12) NOT NULL DEFAULT 'feb28',
  max_attempts integer NOT NULL DEFAULT 3, retry_minutes integer NOT NULL DEFAULT 15,
  birthday_subject varchar(180) NOT NULL, anniversary_subject varchar(180) NOT NULL,
  birthday_template_key varchar(120) NOT NULL DEFAULT 'birthday', anniversary_template_key varchar(120) NOT NULL DEFAULT 'anniversary',
  updated_by_user_id uuid, updated_by_email varchar(255), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT celebration_feb29_policy CHECK (feb29_policy IN ('feb28','mar1','leap_only')),
  CONSTRAINT celebration_max_attempts CHECK (max_attempts BETWEEN 1 AND 10),
  CONSTRAINT celebration_retry_minutes CHECK (retry_minutes BETWEEN 1 AND 1440)
);
INSERT INTO celebration_automation_config(id,enabled,birthday_enabled,anniversary_enabled,timezone,send_time,birthday_subject,anniversary_subject)
VALUES ('default',false,true,true,'Africa/Lagos','09:00','Happy Birthday from The Wisdom Church','Happy Wedding Anniversary from The Wisdom Church') ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS celebration_automation_runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), run_date date NOT NULL UNIQUE, timezone varchar(80) NOT NULL,
  status varchar(20) NOT NULL, attempt integer NOT NULL DEFAULT 1, targeted integer NOT NULL DEFAULT 0,
  sent integer NOT NULL DEFAULT 0, suppressed integer NOT NULL DEFAULT 0, skipped integer NOT NULL DEFAULT 0, failed integer NOT NULL DEFAULT 0,
  last_error text, next_attempt_at timestamptz, claimed_at timestamptz, claimed_by varchar(120), trigger varchar(30) NOT NULL,
  config_snapshot jsonb NOT NULL, started_at timestamptz, completed_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT celebration_run_status CHECK (status IN ('pending','running','partial','completed','failed'))
);
CREATE INDEX IF NOT EXISTS idx_celebration_runs_due ON celebration_automation_runs(run_date,next_attempt_at) WHERE status IN ('pending','partial');

CREATE TABLE IF NOT EXISTS celebration_deliveries (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), run_id uuid NOT NULL REFERENCES celebration_automation_runs(id) ON DELETE CASCADE,
  kind varchar(20) NOT NULL, email_hash char(64) NOT NULL, recipient_email varchar(255) NOT NULL, recipient_name varchar(220) NOT NULL,
  sources jsonb NOT NULL DEFAULT '[]', status varchar(20) NOT NULL, attempt integer NOT NULL DEFAULT 0, error text, sent_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(run_id,kind,email_hash),
  CONSTRAINT celebration_delivery_kind CHECK (kind IN ('birthday','anniversary')),
  CONSTRAINT celebration_delivery_status CHECK (status IN ('pending','sent','suppressed','skipped','failed'))
);
CREATE INDEX IF NOT EXISTS idx_celebration_deliveries_run_status ON celebration_deliveries(run_id,status);
