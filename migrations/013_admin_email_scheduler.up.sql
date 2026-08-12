CREATE TABLE IF NOT EXISTS admin_email_schedules (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), name varchar(160) NOT NULL,
  description varchar(500) NOT NULL DEFAULT '', status varchar(20) NOT NULL DEFAULT 'draft',
  recurrence varchar(20) NOT NULL, timezone varchar(80) NOT NULL, send_time char(5) NOT NULL,
  weekdays jsonb NOT NULL DEFAULT '[]', month_days jsonb NOT NULL DEFAULT '[]',
  start_at timestamptz NOT NULL, end_at timestamptz, next_run_at timestamptz, last_run_at timestamptz,
  compose_payload jsonb NOT NULL, subject varchar(255) NOT NULL, audience_label varchar(255) NOT NULL DEFAULT '',
  run_count integer NOT NULL DEFAULT 0, consecutive_errors integer NOT NULL DEFAULT 0, last_error text,
  claimed_at timestamptz, claimed_by varchar(120), created_by_user_id uuid,
  created_by_email varchar(255), created_by_role varchar(50), created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
  CONSTRAINT admin_email_schedules_status CHECK (status IN ('draft','active','paused','completed','failed')),
  CONSTRAINT admin_email_schedules_recurrence CHECK (recurrence IN ('once','weekly','monthly'))
);
CREATE INDEX IF NOT EXISTS idx_admin_email_schedules_due ON admin_email_schedules(next_run_at) WHERE status = 'active' AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_admin_email_schedules_status ON admin_email_schedules(status);

CREATE TABLE IF NOT EXISTS admin_email_schedule_runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), schedule_id uuid NOT NULL REFERENCES admin_email_schedules(id) ON DELETE CASCADE,
  scheduled_for timestamptz NOT NULL, status varchar(20) NOT NULL, delivery_id uuid REFERENCES admin_email_deliveries(id) ON DELETE SET NULL,
  sent integer NOT NULL DEFAULT 0, failed integer NOT NULL DEFAULT 0, error text,
  started_at timestamptz NOT NULL, completed_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(schedule_id, scheduled_for)
);
CREATE INDEX IF NOT EXISTS idx_admin_email_schedule_runs_schedule ON admin_email_schedule_runs(schedule_id, scheduled_for DESC);
