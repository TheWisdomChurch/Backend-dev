BEGIN;

-- rollback: 020_celebration_automation_config_repair.down.sql
-- Deliberately non-destructive. Rolling back this repair must not delete a
-- production automation configuration that administrators may have updated.
SELECT 1;
-- rollback: 019_form_architecture_and_slug_aliases.down.sql
UPDATE forms SET settings = settings - 'rendererVersion' WHERE settings ? 'rendererVersion';
DROP TABLE IF EXISTS form_slug_aliases;
-- rollback: 018_form_reminder_delivery_claims.down.sql
DROP INDEX IF EXISTS idx_form_reminders_due_delivery;
ALTER TABLE form_calendar_reminders DROP CONSTRAINT IF EXISTS chk_form_reminder_delivery_status;
ALTER TABLE form_calendar_reminders DROP COLUMN IF EXISTS last_error;
ALTER TABLE form_calendar_reminders DROP COLUMN IF EXISTS claimed_by;
ALTER TABLE form_calendar_reminders DROP COLUMN IF EXISTS claimed_at;
ALTER TABLE form_calendar_reminders DROP COLUMN IF EXISTS delivery_attempt;
ALTER TABLE form_calendar_reminders DROP COLUMN IF EXISTS delivery_status;
-- rollback: 017_admin_email_recipient_results.down.sql
DROP INDEX IF EXISTS idx_admin_email_delivery_recipient_results;
ALTER TABLE admin_email_deliveries DROP CONSTRAINT IF EXISTS chk_admin_email_recipient_results_array;
ALTER TABLE admin_email_deliveries DROP COLUMN IF EXISTS recipient_results;
-- rollback: 016_store_checkout_lifecycle.down.sql
DROP INDEX IF EXISTS idx_store_orders_payment_status;
DROP INDEX IF EXISTS idx_store_orders_idempotency_key;
ALTER TABLE store_orders
  DROP CONSTRAINT IF EXISTS chk_store_orders_payment_method,
  DROP CONSTRAINT IF EXISTS chk_store_orders_payment_status,
  DROP CONSTRAINT IF EXISTS chk_store_orders_status,
  DROP COLUMN IF EXISTS reservation_expires_at,
  DROP COLUMN IF EXISTS stock_released_at,
  DROP COLUMN IF EXISTS cancelled_at,
  DROP COLUMN IF EXISTS paid_at,
  DROP COLUMN IF EXISTS payment_slip_url,
  DROP COLUMN IF EXISTS access_token_hash,
  DROP COLUMN IF EXISTS idempotency_key,
  DROP COLUMN IF EXISTS payment_status;
-- rollback: 015_celebration_automation.down.sql
DROP TABLE IF EXISTS celebration_deliveries;
DROP TABLE IF EXISTS celebration_automation_runs;
DROP TABLE IF EXISTS celebration_automation_config;
ALTER TABLE members DROP CONSTRAINT IF EXISTS member_birthday_pair_valid;
ALTER TABLE workforce_members DROP CONSTRAINT IF EXISTS workforce_birthday_pair_valid, DROP CONSTRAINT IF EXISTS workforce_anniversary_pair_valid;
ALTER TABLE leadership_members DROP CONSTRAINT IF EXISTS leadership_birthday_pair_valid, DROP CONSTRAINT IF EXISTS leadership_anniversary_pair_valid;
-- rollback: 014_admin_email_scheduler_hardening.down.sql
ALTER TABLE admin_email_schedule_runs DROP CONSTRAINT IF EXISTS admin_email_schedule_runs_status;
ALTER TABLE admin_email_schedule_runs DROP COLUMN IF EXISTS attempt;
DROP INDEX IF EXISTS idx_admin_email_schedules_pending_occurrence;
ALTER TABLE admin_email_schedules
  DROP COLUMN IF EXISTS version,
  DROP COLUMN IF EXISTS pending_occurrence_at,
  DROP COLUMN IF EXISTS end_date,
  DROP COLUMN IF EXISTS start_date;
-- rollback: 013_admin_email_scheduler.down.sql
DROP TABLE IF EXISTS admin_email_schedule_runs;
DROP TABLE IF EXISTS admin_email_schedules;
-- rollback: 012_visit_workflow.down.sql
-- Destructive manual rollback: removes the visit workflow and all visit data.
-- The application migration runner never executes down migrations automatically.
DROP TABLE IF EXISTS visit_activities;
DROP TABLE IF EXISTS visit_requests;
-- rollback: 011_consolidated_incremental_schema.down.sql
-- Rollback for 011_consolidated_incremental_schema.up.sql — reverses each
-- formerly-separate migration in the opposite order it was applied.
-- Note: the duplicate-row cleanup for trusted_devices (originally part of
-- 001_consolidated_incremental_schema) is not reversible — those rows are
-- gone for good, same as before this consolidation. Likewise the various
-- data backfills in 009 and 010 are not reversed; only schema objects are.

-- Formerly 010_backfill_workforce_dates
DROP INDEX IF EXISTS idx_workforce_anniversary_month_day;
DROP INDEX IF EXISTS idx_workforce_birthday_month_day;
-- Date columns are intentionally retained: dropping them would destroy user data.

-- Formerly 009_normalize_ministry_workforce
DROP TABLE IF EXISTS ministry_workforce_members;

-- Formerly 008_new_member_workflows
DROP TABLE IF EXISTS new_member_workflow_history;
DROP TABLE IF EXISTS new_member_contacts;
DROP TABLE IF EXISTS new_member_workflows;

-- Formerly 007_analytics_pipeline
DROP INDEX IF EXISTS idx_attendance_records_session_active;
DROP INDEX IF EXISTS idx_attendance_sessions_date_active;
DROP INDEX IF EXISTS idx_form_submissions_form_created;
DROP TABLE IF EXISTS analytics_events;
DROP INDEX IF EXISTS idx_analytics_batches_expires_at;
ALTER TABLE analytics_batches DROP COLUMN IF EXISTS expires_at;
DROP INDEX IF EXISTS idx_analytics_batches_batch_id_unique;
CREATE INDEX IF NOT EXISTS idx_analytics_batches_batch_id ON analytics_batches(batch_id);
DROP INDEX IF EXISTS idx_events_category_event_date;
DROP INDEX IF EXISTS idx_events_event_date;
DROP TRIGGER IF EXISTS trg_events_sync_native_date ON events;
DROP FUNCTION IF EXISTS sync_event_native_date();
ALTER TABLE events DROP COLUMN IF EXISTS event_date;

-- Formerly 006_prayer_request_integrity
DROP INDEX IF EXISTS idx_prayer_requests_category_created;
ALTER TABLE prayer_requests DROP CONSTRAINT IF EXISTS chk_prayer_requests_status;

-- Formerly 005_workforce_anniversary
ALTER TABLE workforce_members
  DROP COLUMN IF EXISTS anniversary_month,
  DROP COLUMN IF EXISTS anniversary_day;

-- Formerly 004_approval_request_reason
ALTER TABLE approval_requests DROP COLUMN IF EXISTS reason;

-- Formerly 003_schema_drift_reconciliation
DROP TABLE IF EXISTS store_order_items CASCADE;
DROP TABLE IF EXISTS store_orders CASCADE;
DROP TABLE IF EXISTS store_products CASCADE;
DROP TABLE IF EXISTS analytics_batches CASCADE;
DROP TABLE IF EXISTS ticket_sequences CASCADE;
DROP TABLE IF EXISTS registration_sequences CASCADE;
DROP TABLE IF EXISTS admin_email_deliveries CASCADE;
DROP TABLE IF EXISTS form_campaign_deliveries CASCADE;
DROP TABLE IF EXISTS approval_requests CASCADE;
DROP TABLE IF EXISTS admin_notifications CASCADE;

ALTER TABLE IF EXISTS testimonials DROP COLUMN IF EXISTS approved_at;
ALTER TABLE IF EXISTS testimonials DROP COLUMN IF EXISTS approved_by_email;
ALTER TABLE IF EXISTS testimonials DROP COLUMN IF EXISTS approved_by_name;
ALTER TABLE IF EXISTS testimonials DROP COLUMN IF EXISTS approved_by_id;

ALTER TABLE IF EXISTS events DROP COLUMN IF EXISTS approved_at;
ALTER TABLE IF EXISTS events DROP COLUMN IF EXISTS approved_by_email;
ALTER TABLE IF EXISTS events DROP COLUMN IF EXISTS approved_by_name;
ALTER TABLE IF EXISTS events DROP COLUMN IF EXISTS approved_by_id;
ALTER TABLE IF EXISTS events DROP COLUMN IF EXISTS is_approved;

-- Formerly 002_audit_logs
DROP TABLE IF EXISTS audit_logs CASCADE;

-- Formerly 001_consolidated_incremental_schema
-- Formerly 001/011_ministries
DROP TABLE IF EXISTS ministry_members CASCADE;
DROP TABLE IF EXISTS ministries CASCADE;

-- Formerly 001/010_performance_indexes
DROP INDEX IF EXISTS public.idx_security_events_user_type;
DROP INDEX IF EXISTS public.idx_otps_email_purpose_active;
DROP INDEX IF EXISTS public.idx_members_fts;
DROP INDEX IF EXISTS public.idx_members_birthday;
DROP INDEX IF EXISTS public.idx_trusted_devices_user_device;

-- Formerly 001/009_prayer_requests
DROP TABLE IF EXISTS prayer_requests CASCADE;

-- Formerly 001/008_cell_groups
DROP TABLE IF EXISTS cell_group_meetings CASCADE;
DROP TABLE IF EXISTS cell_group_members CASCADE;
DROP TABLE IF EXISTS cell_groups CASCADE;

-- Formerly 001/007_attendance
DROP TABLE IF EXISTS attendance_records CASCADE;
DROP TABLE IF EXISTS attendance_sessions CASCADE;
DROP TABLE IF EXISTS service_types CASCADE;

-- Formerly 001/006_giving_transactions
DROP TABLE IF EXISTS giving_transactions CASCADE;
DROP TABLE IF EXISTS giving_categories CASCADE;

-- Formerly 001/005_campus
DROP INDEX IF EXISTS public.idx_campuses_active;
DROP INDEX IF EXISTS public.idx_members_campus;
DROP INDEX IF EXISTS public.idx_events_campus;

ALTER TABLE members           DROP COLUMN IF EXISTS campus_id;
ALTER TABLE events            DROP COLUMN IF EXISTS campus_id;
ALTER TABLE workforce_members DROP COLUMN IF EXISTS campus_id;

DROP TABLE IF EXISTS campuses CASCADE;

-- Formerly 001/004_encrypt_member_pii
ALTER TABLE members DROP COLUMN IF EXISTS phone_enc;

-- Formerly 001/003_add_refresh_tokens
DROP TABLE IF EXISTS refresh_tokens CASCADE;

-- Formerly 001/002_add_trusted_devices_constraint
DROP INDEX IF EXISTS public.idx_trusted_devices_user_id;
DROP INDEX IF EXISTS public.idx_trusted_devices_expires_at;

ALTER TABLE trusted_devices
DROP CONSTRAINT IF EXISTS uq_trusted_devices_user_device;

-- Formerly 001/001_add_account_lockout
DROP INDEX IF EXISTS public.idx_users_is_locked;
DROP INDEX IF EXISTS public.idx_users_locked_until;

ALTER TABLE users
DROP COLUMN IF EXISTS failed_login_count,
DROP COLUMN IF EXISTS last_failed_login_at,
DROP COLUMN IF EXISTS is_locked,
DROP COLUMN IF EXISTS locked_until;
-- rollback: schema.down.sql
-- schema.down.sql
-- Rollback consolidated schema
-- Version: v6 (leadership anniversaries + senior pastor role)


DROP INDEX IF EXISTS public.idx_form_submissions_form_id_created_at;
DROP INDEX IF EXISTS public.idx_form_submissions_form_id;
DROP INDEX IF EXISTS public.idx_form_calendar_reminders_due;
DROP INDEX IF EXISTS public.idx_form_calendar_reminders_email;
DROP INDEX IF EXISTS public.idx_form_calendar_reminders_slug;
DROP INDEX IF EXISTS public.idx_form_calendar_reminders_token;
DROP INDEX IF EXISTS public.idx_form_calendar_reminders_submission_id;
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
DROP INDEX IF EXISTS public.idx_forms_report_access_token_unique;
DROP INDEX IF EXISTS public.idx_users_federated_subject_unique;
DROP INDEX IF EXISTS public.idx_members_birthday_month_day;
DROP INDEX IF EXISTS public.idx_leadership_anniversary_month_day;
DROP INDEX IF EXISTS public.idx_leadership_email;
DROP INDEX IF EXISTS public.idx_leadership_status;
DROP INDEX IF EXISTS public.idx_leadership_role_status;
DROP INDEX IF EXISTS public.idx_workforce_bday_month_day;
DROP INDEX IF EXISTS public.idx_workforce_source_channel;
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
DROP INDEX IF EXISTS public.idx_site_contents_key_unique;
DROP INDEX IF EXISTS public.idx_pastoral_care_requests_created_at;
DROP INDEX IF EXISTS public.idx_giving_intents_created_at;

DROP TABLE IF EXISTS public.email_templates CASCADE;
DROP TABLE IF EXISTS public.assets CASCADE;
DROP TABLE IF EXISTS public.form_calendar_reminders CASCADE;
DROP TABLE IF EXISTS public.form_submissions CASCADE;
DROP TABLE IF EXISTS public.form_fields CASCADE;
DROP TABLE IF EXISTS public.forms CASCADE;
DROP TABLE IF EXISTS public.members CASCADE;
DROP TABLE IF EXISTS public.leadership_members CASCADE;
DROP TABLE IF EXISTS public.workforce_members CASCADE;
DROP TABLE IF EXISTS public.notification_deliveries CASCADE;
DROP TABLE IF EXISTS public.notifications CASCADE;
DROP TABLE IF EXISTS public.subscribers CASCADE;
DROP TABLE IF EXISTS public.site_contents CASCADE;
DROP TABLE IF EXISTS public.pastoral_care_requests CASCADE;
DROP TABLE IF EXISTS public.giving_intents CASCADE;
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
