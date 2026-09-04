-- migration: 021_wedding_anniversaries.up.sql
-- One record per marriage as it concerns the church: who we greet, when their
-- wedding anniversary falls, who they are married to (member or external), and
-- whether we are still allowed to reach out. Replaces the scattered
-- leadership_members/workforce_members anniversary_month/day fields as the
-- single source for the "anniversary" celebration kind. The legacy columns are
-- left in place and backfilled below; a later migration can drop them.

CREATE TABLE IF NOT EXISTS wedding_anniversaries (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  subject_type varchar(20) NOT NULL,
  subject_id uuid NOT NULL,
  anniversary_month smallint NOT NULL,
  anniversary_day smallint NOT NULL,
  wedding_year smallint,
  spouse_name varchar(200) NOT NULL DEFAULT '',
  spouse_email varchar(255),
  spouse_subject_type varchar(20),
  spouse_subject_id uuid,
  spouse_is_external boolean NOT NULL DEFAULT false,
  spouse_email_consent boolean NOT NULL DEFAULT false,
  status varchar(20) NOT NULL DEFAULT 'active',
  source varchar(20) NOT NULL DEFAULT 'admin',
  source_submission_id uuid,
  notes text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT wedding_anniversary_subject_type CHECK (subject_type IN ('member','leadership','workforce')),
  CONSTRAINT wedding_anniversary_spouse_subject_type CHECK (spouse_subject_type IS NULL OR spouse_subject_type IN ('member','leadership','workforce')),
  CONSTRAINT wedding_anniversary_status CHECK (status IN ('active','archived')),
  CONSTRAINT wedding_anniversary_source CHECK (source IN ('admin','form','import','csv')),
  CONSTRAINT wedding_anniversary_month_valid CHECK (anniversary_month BETWEEN 1 AND 12),
  CONSTRAINT wedding_anniversary_day_valid CHECK (
    anniversary_day BETWEEN 1 AND CASE anniversary_month
      WHEN 2 THEN 29 WHEN 4 THEN 30 WHEN 6 THEN 30 WHEN 9 THEN 30 WHEN 11 THEN 30 ELSE 31 END
  )
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_wedding_anniversary_subject
  ON wedding_anniversaries(subject_type, subject_id);
CREATE INDEX IF NOT EXISTS idx_wedding_anniversary_due
  ON wedding_anniversaries(status, anniversary_month, anniversary_day);

-- Backfill from the legacy inline fields. spouse_name is unknown for these
-- rows; it is left blank and surfaced in admin as "needs spouse details".
INSERT INTO wedding_anniversaries (subject_type, subject_id, anniversary_month, anniversary_day, source, status)
SELECT 'leadership', id, anniversary_month, anniversary_day, 'import', 'active'
FROM leadership_members
WHERE anniversary_month IS NOT NULL AND anniversary_day IS NOT NULL
ON CONFLICT (subject_type, subject_id) DO NOTHING;

INSERT INTO wedding_anniversaries (subject_type, subject_id, anniversary_month, anniversary_day, source, status)
SELECT 'workforce', id, anniversary_month, anniversary_day, 'import', 'active'
FROM workforce_members
WHERE anniversary_month IS NOT NULL AND anniversary_day IS NOT NULL
ON CONFLICT (subject_type, subject_id) DO NOTHING;
