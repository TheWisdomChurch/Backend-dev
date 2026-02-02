-- Adds per-field validation rules JSON column
ALTER TABLE public.form_fields
  ADD COLUMN validation jsonb;
