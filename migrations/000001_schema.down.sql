-- 000001_schema.down.sql
-- Rollback for consolidated baseline schema (golang-migrate compatible)

BEGIN;

-- Remove triggers
DROP TRIGGER IF EXISTS update_users_updated_at ON public.users;
DROP TRIGGER IF EXISTS update_events_updated_at ON public.events;
DROP TRIGGER IF EXISTS update_forms_updated_at ON public.forms;
DROP TRIGGER IF EXISTS update_form_fields_updated_at ON public.form_fields;
DROP TRIGGER IF EXISTS update_form_submissions_updated_at ON public.form_submissions;
DROP TRIGGER IF EXISTS update_notification_deliveries_updated_at ON public.notification_deliveries;
DROP TRIGGER IF EXISTS update_otps_updated_at ON public.otps;
DROP TRIGGER IF EXISTS update_reels_updated_at ON public.reels;
DROP TRIGGER IF EXISTS update_security_events_updated_at ON public.security_events;
DROP TRIGGER IF EXISTS update_subscribers_updated_at ON public.subscribers;
DROP TRIGGER IF EXISTS update_testimonials_updated_at ON public.testimonials;
DROP TRIGGER IF EXISTS update_trusted_devices_updated_at ON public.trusted_devices;
DROP TRIGGER IF EXISTS update_workforce_members_updated_at ON public.workforce_members;
DROP TRIGGER IF EXISTS update_members_updated_at ON public.members;

-- Drop tables (reverse dependency order)
DROP TABLE IF EXISTS public.notification_deliveries;
DROP TABLE IF EXISTS public.form_fields;
DROP TABLE IF EXISTS public.form_submissions;
DROP TABLE IF EXISTS public.forms;
DROP TABLE IF EXISTS public.reels;
DROP TABLE IF EXISTS public.notifications;
DROP TABLE IF EXISTS public.otps;
DROP TABLE IF EXISTS public.security_events;
DROP TABLE IF EXISTS public.trusted_devices;
DROP TABLE IF EXISTS public.subscribers;
DROP TABLE IF EXISTS public.testimonials;
DROP TABLE IF EXISTS public.members;
DROP TABLE IF EXISTS public.workforce_members;
DROP TABLE IF EXISTS public.events;
DROP TABLE IF EXISTS public.users;

-- Cleanup helper function
DROP FUNCTION IF EXISTS public.update_updated_at_column();

-- Drop extensions added by the up migration
DROP EXTENSION IF EXISTS "uuid-ossp";
DROP EXTENSION IF EXISTS "pgcrypto";

COMMIT;
