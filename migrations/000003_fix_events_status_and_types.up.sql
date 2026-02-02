-- 000003_fix_events_status_and_types.up.sql
-- Align events table with application semantics:
-- - status values: upcoming | happening | past
-- - store date/time as varchar to match JSON payloads and GORM model

BEGIN;

-- Drop old constraint (it expected draft/scheduled/etc.)
ALTER TABLE public.events
  DROP CONSTRAINT IF EXISTS events_status_check;

-- Normalize existing rows to the new lifecycle values based on date
UPDATE public.events
SET status = CASE
    WHEN "date"::text ~ '^\d{4}-\d{2}-\d{2}$' AND to_date("date"::text, 'YYYY-MM-DD') = CURRENT_DATE THEN 'happening'
    WHEN "date"::text ~ '^\d{4}-\d{2}-\d{2}$' AND to_date("date"::text, 'YYYY-MM-DD') > CURRENT_DATE THEN 'upcoming'
    WHEN status IN ('upcoming', 'happening', 'past') THEN status
    ELSE 'past'
  END;

-- Make column types match the Go model (varchar) and set a safe default
ALTER TABLE public.events
  ALTER COLUMN "date" TYPE varchar(20) USING "date"::text,
  ALTER COLUMN "time" TYPE varchar(50) USING "time"::text,
  ALTER COLUMN status TYPE varchar(20) USING status::text,
  ALTER COLUMN status SET DEFAULT 'upcoming';

-- Recreate constraint with the allowed values the API uses
ALTER TABLE public.events
  ADD CONSTRAINT events_status_check
  CHECK (status IN ('upcoming', 'happening', 'past'));

COMMIT;
