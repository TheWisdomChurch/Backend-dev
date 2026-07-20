-- Rollback for 001_consolidated_incremental_schema.up.sql — reverses each
-- formerly-numbered migration in the opposite order it was applied.
-- Note: the duplicate-row cleanup in the former 002 (trusted_devices) is not
-- reversible — those rows are gone for good, same as before consolidation.

-- Formerly 011_ministries
DROP TABLE IF EXISTS ministry_members CASCADE;
DROP TABLE IF EXISTS ministries CASCADE;

-- Formerly 010_performance_indexes
DROP INDEX IF EXISTS public.idx_security_events_user_type;
DROP INDEX IF EXISTS public.idx_otps_email_purpose_active;
DROP INDEX IF EXISTS public.idx_members_fts;
DROP INDEX IF EXISTS public.idx_members_birthday;
DROP INDEX IF EXISTS public.idx_trusted_devices_user_device;

-- Formerly 009_prayer_requests
DROP TABLE IF EXISTS prayer_requests CASCADE;

-- Formerly 008_cell_groups
DROP TABLE IF EXISTS cell_group_meetings CASCADE;
DROP TABLE IF EXISTS cell_group_members CASCADE;
DROP TABLE IF EXISTS cell_groups CASCADE;

-- Formerly 007_attendance
DROP TABLE IF EXISTS attendance_records CASCADE;
DROP TABLE IF EXISTS attendance_sessions CASCADE;
DROP TABLE IF EXISTS service_types CASCADE;

-- Formerly 006_giving_transactions
DROP TABLE IF EXISTS giving_transactions CASCADE;
DROP TABLE IF EXISTS giving_categories CASCADE;

-- Formerly 005_campus
DROP INDEX IF EXISTS public.idx_campuses_active;
DROP INDEX IF EXISTS public.idx_members_campus;
DROP INDEX IF EXISTS public.idx_events_campus;

ALTER TABLE members           DROP COLUMN IF EXISTS campus_id;
ALTER TABLE events            DROP COLUMN IF EXISTS campus_id;
ALTER TABLE workforce_members DROP COLUMN IF EXISTS campus_id;

DROP TABLE IF EXISTS campuses CASCADE;

-- Formerly 004_encrypt_member_pii
ALTER TABLE members DROP COLUMN IF EXISTS phone_enc;

-- Formerly 003_add_refresh_tokens
DROP TABLE IF EXISTS refresh_tokens CASCADE;

-- Formerly 002_add_trusted_devices_constraint
DROP INDEX IF EXISTS public.idx_trusted_devices_user_id;
DROP INDEX IF EXISTS public.idx_trusted_devices_expires_at;

ALTER TABLE trusted_devices
DROP CONSTRAINT IF EXISTS uq_trusted_devices_user_device;

-- Formerly 001_add_account_lockout
DROP INDEX IF EXISTS public.idx_users_is_locked;
DROP INDEX IF EXISTS public.idx_users_locked_until;

ALTER TABLE users
DROP COLUMN IF EXISTS failed_login_count,
DROP COLUMN IF EXISTS last_failed_login_at,
DROP COLUMN IF EXISTS is_locked,
DROP COLUMN IF EXISTS locked_until;
