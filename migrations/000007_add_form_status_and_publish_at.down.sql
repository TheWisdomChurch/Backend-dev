-- 000007_add_form_status_and_publish_at.down.sql
-- Revert status/published_at changes and restore unique title constraint

CREATE UNIQUE INDEX IF NOT EXISTS idx_forms_title_unique
  ON public.forms (LOWER(title))
  WHERE deleted_at IS NULL;

ALTER TABLE public.forms
  DROP COLUMN IF EXISTS published_at,
  DROP COLUMN IF EXISTS status;
