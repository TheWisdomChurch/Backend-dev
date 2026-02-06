BEGIN;

DROP TABLE IF EXISTS public.registration_sequences;
DROP TABLE IF EXISTS public.ticket_sequences;
DROP TABLE IF EXISTS public.admin_notifications;
DROP TABLE IF EXISTS public.approval_requests;

DROP INDEX IF EXISTS idx_form_submissions_registration_code;
ALTER TABLE public.form_submissions
  DROP COLUMN IF EXISTS registration_code;

ALTER TABLE public.testimonials
  DROP COLUMN IF EXISTS approved_by_id,
  DROP COLUMN IF EXISTS approved_by_name,
  DROP COLUMN IF EXISTS approved_by_email,
  DROP COLUMN IF EXISTS approved_at;

ALTER TABLE public.events
  DROP COLUMN IF EXISTS is_approved,
  DROP COLUMN IF EXISTS approved_by_id,
  DROP COLUMN IF EXISTS approved_by_name,
  DROP COLUMN IF EXISTS approved_by_email,
  DROP COLUMN IF EXISTS approved_at;

COMMIT;
