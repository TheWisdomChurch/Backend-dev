-- Add birthday month/day fields for workforce members and index for monthly grouping
BEGIN;

ALTER TABLE public.workforce_members
  ADD COLUMN birthday_month smallint CHECK (birthday_month BETWEEN 1 AND 12),
  ADD COLUMN birthday_day   smallint CHECK (birthday_day BETWEEN 1 AND 31);

CREATE INDEX IF NOT EXISTS idx_workforce_bday_month_day
  ON public.workforce_members (birthday_month, birthday_day);

COMMIT;
