BEGIN;

-- Events approvals
ALTER TABLE public.events
  ADD COLUMN IF NOT EXISTS is_approved boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS approved_by_id uuid,
  ADD COLUMN IF NOT EXISTS approved_by_name character varying(120),
  ADD COLUMN IF NOT EXISTS approved_by_email character varying(255),
  ADD COLUMN IF NOT EXISTS approved_at timestamp with time zone;

UPDATE public.events SET is_approved = true WHERE is_approved = false;

-- Testimonials approvals
ALTER TABLE public.testimonials
  ADD COLUMN IF NOT EXISTS approved_by_id uuid,
  ADD COLUMN IF NOT EXISTS approved_by_name character varying(120),
  ADD COLUMN IF NOT EXISTS approved_by_email character varying(255),
  ADD COLUMN IF NOT EXISTS approved_at timestamp with time zone;

-- Form submissions registration code
ALTER TABLE public.form_submissions
  ADD COLUMN IF NOT EXISTS registration_code character varying(40);

CREATE UNIQUE INDEX IF NOT EXISTS idx_form_submissions_registration_code
  ON public.form_submissions (registration_code)
  WHERE registration_code IS NOT NULL;

-- Approval requests (tickets)
CREATE TABLE IF NOT EXISTS public.approval_requests (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  ticket_code character varying(50) NOT NULL,
  type character varying(30) NOT NULL,
  status character varying(20) NOT NULL DEFAULT 'pending',
  entity_id uuid,
  entity_label character varying(255),
  requested_by_id uuid,
  requested_by_name character varying(120),
  requested_by_email character varying(255),
  approved_by_id uuid,
  approved_by_name character varying(120),
  approved_by_email character varying(255),
  approved_at timestamp with time zone,
  created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_approval_requests_ticket
  ON public.approval_requests (ticket_code);
CREATE INDEX IF NOT EXISTS idx_approval_requests_status
  ON public.approval_requests (status);
CREATE INDEX IF NOT EXISTS idx_approval_requests_type
  ON public.approval_requests (type);
CREATE INDEX IF NOT EXISTS idx_approval_requests_created_at
  ON public.approval_requests (created_at DESC);

-- Admin notifications (in-app)
CREATE TABLE IF NOT EXISTS public.admin_notifications (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL,
  type character varying(40) NOT NULL,
  title character varying(255) NOT NULL,
  message text NOT NULL,
  ticket_code character varying(50),
  entity_type character varying(40),
  entity_id uuid,
  is_read boolean NOT NULL DEFAULT false,
  read_at timestamp with time zone,
  created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_admin_notifications_user
  ON public.admin_notifications (user_id);
CREATE INDEX IF NOT EXISTS idx_admin_notifications_read
  ON public.admin_notifications (is_read);
CREATE INDEX IF NOT EXISTS idx_admin_notifications_created_at
  ON public.admin_notifications (created_at DESC);

-- Ticket sequences
CREATE TABLE IF NOT EXISTS public.ticket_sequences (
  prefix character varying(40) PRIMARY KEY,
  last_number integer NOT NULL DEFAULT 0,
  updated_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Registration sequences
CREATE TABLE IF NOT EXISTS public.registration_sequences (
  prefix character varying(20) PRIMARY KEY,
  last_number integer NOT NULL DEFAULT 0,
  updated_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
);

COMMIT;
