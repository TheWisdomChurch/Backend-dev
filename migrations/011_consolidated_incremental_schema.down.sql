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
