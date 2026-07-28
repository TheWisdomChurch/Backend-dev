-- Ensure drifted production schemas have the recurring date columns even when
-- they were created before the current baseline/005 migration history.
ALTER TABLE workforce_members
  ADD COLUMN IF NOT EXISTS birthday_month smallint,
  ADD COLUMN IF NOT EXISTS birthday_day smallint,
  ADD COLUMN IF NOT EXISTS anniversary_month smallint,
  ADD COLUMN IF NOT EXISTS anniversary_day smallint;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'workforce_birthday_month_check') THEN
    ALTER TABLE workforce_members ADD CONSTRAINT workforce_birthday_month_check CHECK (birthday_month BETWEEN 1 AND 12);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'workforce_birthday_day_check') THEN
    ALTER TABLE workforce_members ADD CONSTRAINT workforce_birthday_day_check CHECK (birthday_day BETWEEN 1 AND 31);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'workforce_anniversary_month_check') THEN
    ALTER TABLE workforce_members ADD CONSTRAINT workforce_anniversary_month_check CHECK (anniversary_month BETWEEN 1 AND 12);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'workforce_anniversary_day_check') THEN
    ALTER TABLE workforce_members ADD CONSTRAINT workforce_anniversary_day_check CHECK (anniversary_day BETWEEN 1 AND 31);
  END IF;
END $$;

-- Backfill only deterministic workforce submissions matched by normalized
-- email. Existing administrator-corrected values always win. Accepted stored
-- shapes are ISO YYYY-MM-DD and DD/MM[/YYYY] (also '-' or '.' separators).
WITH raw AS (
  SELECT fs.created_at, lower(trim(fs.email)) AS email,
    COALESCE(NULLIF(trim(fs.values->>'birthday'), ''), NULLIF(trim(fs.values->>'birthDate'), ''), NULLIF(trim(fs.values->>'birth_date'), ''), NULLIF(trim(fs.values->>'dob'), ''), NULLIF(trim(fs.values->>'dateOfBirth'), ''), NULLIF(trim(fs.values->>'date_of_birth'), '')) AS birthday,
    COALESCE(NULLIF(trim(fs.values->>'anniversary'), ''), NULLIF(trim(fs.values->>'weddingAnniversary'), ''), NULLIF(trim(fs.values->>'wedding_anniversary'), ''), NULLIF(trim(fs.values->>'anniversaryDate'), ''), NULLIF(trim(fs.values->>'anniversary_date'), '')) AS anniversary
  FROM form_submissions fs
  JOIN forms f ON f.id = fs.form_id AND f.deleted_at IS NULL
  WHERE fs.deleted_at IS NULL AND trim(COALESCE(fs.email, '')) <> ''
    AND (lower(COALESCE(f.settings->>'submissionTarget', '')) LIKE 'workforce%' OR lower(COALESCE(f.settings->>'formType', '')) = 'workforce' OR lower(COALESCE(f.slug, '')) LIKE '%workforce%')
), parsed AS (
  SELECT *,
    CASE WHEN birthday ~ '^\d{4}-\d{1,2}-\d{1,2}$' THEN split_part(birthday, '-', 2)::int WHEN birthday ~ '^\d{1,2}[/.-]\d{1,2}([/.-]\d{2,4})?$' THEN regexp_replace(birthday, '^\d{1,2}[/.-](\d{1,2}).*$', '\1')::int END AS bm,
    CASE WHEN birthday ~ '^\d{4}-\d{1,2}-\d{1,2}$' THEN split_part(birthday, '-', 3)::int WHEN birthday ~ '^\d{1,2}[/.-]\d{1,2}([/.-]\d{2,4})?$' THEN regexp_replace(birthday, '^(\d{1,2})[/.-].*$', '\1')::int END AS bd,
    CASE WHEN anniversary ~ '^\d{4}-\d{1,2}-\d{1,2}$' THEN split_part(anniversary, '-', 2)::int WHEN anniversary ~ '^\d{1,2}[/.-]\d{1,2}([/.-]\d{2,4})?$' THEN regexp_replace(anniversary, '^\d{1,2}[/.-](\d{1,2}).*$', '\1')::int END AS am,
    CASE WHEN anniversary ~ '^\d{4}-\d{1,2}-\d{1,2}$' THEN split_part(anniversary, '-', 3)::int WHEN anniversary ~ '^\d{1,2}[/.-]\d{1,2}([/.-]\d{2,4})?$' THEN regexp_replace(anniversary, '^(\d{1,2})[/.-].*$', '\1')::int END AS ad
  FROM raw
), valid AS (
  SELECT *,
    (bm BETWEEN 1 AND 12 AND bd BETWEEN 1 AND CASE bm WHEN 2 THEN 29 WHEN 4 THEN 30 WHEN 6 THEN 30 WHEN 9 THEN 30 WHEN 11 THEN 30 ELSE 31 END) AS birthday_valid,
    (am BETWEEN 1 AND 12 AND ad BETWEEN 1 AND CASE am WHEN 2 THEN 29 WHEN 4 THEN 30 WHEN 6 THEN 30 WHEN 9 THEN 30 WHEN 11 THEN 30 ELSE 31 END) AS anniversary_valid
  FROM parsed
), latest_birthday AS (
  SELECT DISTINCT ON (email) email, bm, bd FROM valid WHERE birthday_valid ORDER BY email, created_at DESC
), latest_anniversary AS (
  SELECT DISTINCT ON (email) email, am, ad FROM valid WHERE anniversary_valid ORDER BY email, created_at DESC
), dates AS (
  SELECT COALESCE(b.email, a.email) AS email, b.bm, b.bd, a.am, a.ad
  FROM latest_birthday b FULL OUTER JOIN latest_anniversary a ON a.email = b.email
)
UPDATE workforce_members w SET
  birthday_month = COALESCE(w.birthday_month, d.bm),
  birthday_day = COALESCE(w.birthday_day, d.bd),
  anniversary_month = COALESCE(w.anniversary_month, d.am),
  anniversary_day = COALESCE(w.anniversary_day, d.ad),
  updated_at = CASE WHEN (w.birthday_month IS NULL AND d.bm IS NOT NULL) OR (w.anniversary_month IS NULL AND d.am IS NOT NULL) THEN now() ELSE w.updated_at END
FROM dates d WHERE lower(trim(w.email)) = d.email;

CREATE INDEX IF NOT EXISTS idx_workforce_birthday_month_day ON workforce_members(birthday_month, birthday_day) WHERE birthday_month IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_workforce_anniversary_month_day ON workforce_members(anniversary_month, anniversary_day) WHERE anniversary_month IS NOT NULL;
