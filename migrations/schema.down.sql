-- schema.down.sql
-- Rollback consolidated schema
-- Version: v4 (assets + email_templates)

BEGIN;

DROP INDEX IF EXISTS public.idx_form_submissions_form_id_created_at;
DROP INDEX IF EXISTS public.idx_form_submissions_form_id;
DROP INDEX IF EXISTS public.idx_email_templates_key_version_unique;
DROP INDEX IF EXISTS public.idx_email_templates_key;
DROP INDEX IF EXISTS public.idx_email_templates_owner;
DROP INDEX IF EXISTS public.idx_assets_status;
DROP INDEX IF EXISTS public.idx_assets_kind;
DROP INDEX IF EXISTS public.idx_assets_owner;
DROP INDEX IF EXISTS public.idx_assets_object_key_unique;
DROP INDEX IF EXISTS public.idx_form_fields_form_id_order;
DROP INDEX IF EXISTS public.idx_form_fields_form_id;
DROP INDEX IF EXISTS public.idx_forms_status;
DROP INDEX IF EXISTS public.idx_forms_event_id;
DROP INDEX IF EXISTS public.idx_members_birthday_month_day;
DROP INDEX IF EXISTS public.idx_workforce_bday_month_day;
DROP INDEX IF EXISTS public.idx_notification_deliveries_subscriber_id;
DROP INDEX IF EXISTS public.idx_notification_deliveries_notification_id;
DROP INDEX IF EXISTS public.idx_notifications_event_id;
DROP INDEX IF EXISTS public.idx_reels_event_id;
DROP INDEX IF EXISTS public.idx_events_date;
DROP INDEX IF EXISTS public.idx_events_status;
DROP INDEX IF EXISTS public.idx_otps_purpose;
DROP INDEX IF EXISTS public.idx_otps_email;
DROP INDEX IF EXISTS public.idx_security_events_email;
DROP INDEX IF EXISTS public.idx_security_events_user_id;
DROP INDEX IF EXISTS public.idx_trusted_devices_device_id;
DROP INDEX IF EXISTS public.idx_trusted_devices_user_id;
DROP INDEX IF EXISTS public.idx_forms_slug_unique;
DROP INDEX IF EXISTS public.idx_subscribers_email_unique;
DROP INDEX IF EXISTS public.idx_users_email_unique;

DROP TABLE IF EXISTS public.email_templates CASCADE;
DROP TABLE IF EXISTS public.assets CASCADE;
DROP TABLE IF EXISTS public.form_submissions CASCADE;
DROP TABLE IF EXISTS public.form_fields CASCADE;
DROP TABLE IF EXISTS public.forms CASCADE;
DROP TABLE IF EXISTS public.members CASCADE;
DROP TABLE IF EXISTS public.workforce_members CASCADE;
DROP TABLE IF EXISTS public.notification_deliveries CASCADE;
DROP TABLE IF EXISTS public.notifications CASCADE;
DROP TABLE IF EXISTS public.subscribers CASCADE;
DROP TABLE IF EXISTS public.testimonials CASCADE;
DROP TABLE IF EXISTS public.reels CASCADE;
DROP TABLE IF EXISTS public.events CASCADE;
DROP TABLE IF EXISTS public.otps CASCADE;
DROP TABLE IF EXISTS public.security_events CASCADE;
DROP TABLE IF EXISTS public.trusted_devices CASCADE;
DROP TABLE IF EXISTS public.users CASCADE;

DROP FUNCTION IF EXISTS public.update_updated_at_column();

DROP EXTENSION IF EXISTS "uuid-ossp";
DROP EXTENSION IF EXISTS "pgcrypto";

COMMIT;
