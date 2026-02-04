-- Drops per-field visibility rules JSON column
ALTER TABLE public.form_fields
  DROP COLUMN IF EXISTS visibility;
