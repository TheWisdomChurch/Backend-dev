-- 000002_forms_unique_title.up.sql
-- Ensure form titles are unique (case-insensitive) for non-deleted forms

CREATE UNIQUE INDEX IF NOT EXISTS idx_forms_title_unique
  ON public.forms (LOWER(title))
  WHERE deleted_at IS NULL;
