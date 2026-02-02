BEGIN;
DROP INDEX IF EXISTS idx_workforce_bday_month_day;
ALTER TABLE public.workforce_members
  DROP COLUMN IF EXISTS birthday_month,
  DROP COLUMN IF EXISTS birthday_day;
COMMIT;
