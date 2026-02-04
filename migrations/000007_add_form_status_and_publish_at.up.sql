-- 000007_add_form_status_and_publish_at.up.sql
-- Add status/published_at to forms and relax title uniqueness

ALTER TABLE public.forms
  ADD COLUMN IF NOT EXISTS status character varying(20) NOT NULL DEFAULT 'draft',
  ADD COLUMN IF NOT EXISTS published_at timestamp with time zone;

-- Backfill status for existing rows
UPDATE public.forms
SET status = CASE WHEN is_published THEN 'published' ELSE 'draft' END
WHERE status IS NULL OR status = '';

-- Backfill published_at for existing published forms
UPDATE public.forms
SET published_at = COALESCE(published_at, updated_at, created_at)
WHERE is_published = true AND published_at IS NULL;

-- Allow duplicate titles (unique ID is the primary key)
DROP INDEX IF EXISTS public.idx_forms_title_unique;
