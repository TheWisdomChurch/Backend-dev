-- 000003_fix_events_status_and_types.down.sql
-- Revert events table changes (back to original constraint and date/time types)

BEGIN;

ALTER TABLE public.events
  DROP CONSTRAINT IF EXISTS events_status_check;

-- Map simplified lifecycle back to the previous states (best-effort)
UPDATE public.events
SET status = CASE
    WHEN status = 'happening' THEN 'published'
    WHEN status = 'upcoming' THEN 'scheduled'
    WHEN status = 'past' THEN 'completed'
    ELSE 'draft'
  END;

-- Convert columns back to date/time; fallback to CURRENT_DATE/00:00 to avoid invalid casts
ALTER TABLE public.events
  ALTER COLUMN "date" TYPE date USING (
    CASE
      WHEN "date"::text ~ '^\d{4}-\d{2}-\d{2}$' THEN to_date("date"::text, 'YYYY-MM-DD')
      ELSE CURRENT_DATE
    END
  ),
  ALTER COLUMN "time" TYPE time without time zone USING (
    CASE
      WHEN "time"::text ~ '^\d{2}:\d{2}(:\d{2})?$' THEN "time"::time
      ELSE TIME '00:00'
    END
  ),
  ALTER COLUMN status TYPE varchar(20) USING status::text,
  ALTER COLUMN status DROP DEFAULT;

ALTER TABLE public.events
  ADD CONSTRAINT events_status_check
  CHECK (status IN ('draft','scheduled','published','cancelled','completed','archived'));

COMMIT;
