-- Removes per-field validation rules column
ALTER TABLE public.form_fields
  DROP COLUMN IF EXISTS validation;
