-- Adds per-field visibility rules JSON column
ALTER TABLE public.form_fields
  ADD COLUMN visibility jsonb;
