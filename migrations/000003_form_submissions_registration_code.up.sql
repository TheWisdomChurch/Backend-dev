ALTER TABLE public.form_submissions
  ADD COLUMN IF NOT EXISTS registration_code character varying(40);
