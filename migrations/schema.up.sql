-- migration: schema.up.sql
-- schema.up.sql
-- Consolidated, idempotent schema (industry-standard constraints, indexes, and FKs)
-- Version: v6 (leadership anniversaries + senior pastor role)


CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE OR REPLACE FUNCTION public.update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = CURRENT_TIMESTAMP;
  RETURN NEW;
END;
$$ language 'plpgsql';

-- =========================
-- CORE TABLES
-- =========================

CREATE TABLE IF NOT EXISTS public.users (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  first_name character varying(100) NOT NULL,
  last_name character varying(100) NOT NULL,
  email character varying(255) NOT NULL,
  password character varying(255) NOT NULL,
  federated_provider character varying(50),
  federated_subject character varying(255),
  federated_linked_at timestamp with time zone,
  preferred_mfa_method character varying(30) DEFAULT 'email_otp' NOT NULL,
  totp_enabled boolean DEFAULT false NOT NULL,
  totp_secret_enc text,
  totp_pending_enc text,
  role character varying(50) DEFAULT 'admin'::character varying NOT NULL,
  is_active boolean DEFAULT true NOT NULL,
  admin_approved boolean DEFAULT true NOT NULL,
  failed_login_count bigint DEFAULT 0 NOT NULL,
  last_failed_login_at timestamp with time zone,
  last_login_at timestamp with time zone,
  created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  deleted_at timestamp with time zone,
  CONSTRAINT users_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS public.trusted_devices (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  user_id uuid NOT NULL,
  device_id character varying(255) NOT NULL,
  label character varying(255),
  last_ip character varying(45),
  user_agent character varying(512),
  trusted boolean DEFAULT true NOT NULL,
  expires_at timestamp with time zone,
  last_seen_at timestamp with time zone,
  created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  deleted_at timestamp with time zone,
  CONSTRAINT trusted_devices_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS public.security_events (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  user_id uuid,
  email character varying(255),
  type character varying(100) NOT NULL,
  ip character varying(45),
  user_agent character varying(512),
  metadata jsonb,
  created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  deleted_at timestamp with time zone,
  CONSTRAINT security_events_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS public.otps (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  email character varying(255) NOT NULL,
  purpose character varying(120),
  code_hash character varying(64) NOT NULL,
  code_salt character varying(32) NOT NULL,
  expires_at timestamp with time zone NOT NULL,
  used_at timestamp with time zone,
  created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  deleted_at timestamp with time zone,
  CONSTRAINT otps_pkey PRIMARY KEY (id)
);

-- =========================
-- EVENTS / CONTENT
-- =========================

CREATE TABLE IF NOT EXISTS public.events (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  title character varying(200) NOT NULL,
  short_description character varying(255) NOT NULL,
  description text NOT NULL,
  date character varying(20) NOT NULL,
  "time" character varying(50) NOT NULL,
  location character varying(255) NOT NULL,
  category character varying(30) NOT NULL,
  status character varying(20) NOT NULL DEFAULT 'upcoming',
  is_featured boolean DEFAULT false NOT NULL,
  tags text[] DEFAULT '{}'::text[],
  register_link text,
  speaker character varying(120),
  contact_phone character varying(40),
  image text,
  banner_image text,
  image_key text,
  banner_image_key text,
  attendees bigint DEFAULT 0 NOT NULL,
  created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  CONSTRAINT events_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS public.reels (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  title character varying(200) NOT NULL,
  thumbnail text NOT NULL,
  video_url text NOT NULL,
  duration interval DEFAULT '0 seconds'::interval NOT NULL,
  event_id uuid,
  created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  CONSTRAINT reels_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS public.testimonials (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  first_name character varying(100) NOT NULL,
  last_name character varying(100) NOT NULL,
  full_name character varying(200) GENERATED ALWAYS AS ((((first_name)::text || ' '::text) || (last_name)::text)) STORED,
  image_url character varying(500),
  testimony text NOT NULL,
  is_anonymous boolean DEFAULT false,
  is_approved boolean DEFAULT false,
  created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  deleted_at timestamp with time zone,
  CONSTRAINT testimonials_pkey PRIMARY KEY (id)
);

-- =========================
-- SUBSCRIBERS / NOTIFICATIONS
-- =========================

CREATE TABLE IF NOT EXISTS public.subscribers (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  email character varying(255) NOT NULL,
  name character varying(120),
  source character varying(120),
  status character varying(20) DEFAULT 'active'::character varying NOT NULL,
  created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  deleted_at timestamp with time zone,
  CONSTRAINT subscribers_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS public.notifications (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  type character varying(20) NOT NULL,
  subject character varying(255) NOT NULL,
  title character varying(255) NOT NULL,
  message text NOT NULL,
  event_id uuid,
  created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  CONSTRAINT notifications_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS public.notification_deliveries (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  notification_id uuid NOT NULL,
  subscriber_id uuid NOT NULL,
  status character varying(20) DEFAULT 'pending'::character varying NOT NULL,
  error_message text,
  sent_at timestamp with time zone,
  created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  deleted_at timestamp with time zone,
  CONSTRAINT notification_deliveries_pkey PRIMARY KEY (id)
);

-- =========================
-- CONTENT + ENGAGEMENT INTAKE
-- =========================

CREATE TABLE IF NOT EXISTS public.site_contents (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  key character varying(120) NOT NULL,
  payload jsonb NOT NULL,
  updated_by character varying(255),
  created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  CONSTRAINT site_contents_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS public.pastoral_care_requests (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  title character varying(40) NOT NULL,
  first_name character varying(120) NOT NULL,
  last_name character varying(120) NOT NULL,
  phone character varying(60) NOT NULL,
  email character varying(255) NOT NULL,
  address character varying(500) NOT NULL,
  event_date character varying(20) NOT NULL,
  event_type character varying(120) NOT NULL,
  church_role character varying(120) NOT NULL,
  custom_role character varying(120),
  comments text,
  source_channel character varying(120) NOT NULL DEFAULT 'frontend:web:pastoral-care',
  metadata jsonb,
  created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  CONSTRAINT pastoral_care_requests_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS public.giving_intents (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  title character varying(200) NOT NULL,
  description text,
  source_channel character varying(120) NOT NULL DEFAULT 'frontend:web:online-giving',
  metadata jsonb,
  created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  CONSTRAINT giving_intents_pkey PRIMARY KEY (id)
);

-- =========================
-- WORKFORCE / MEMBERS
-- =========================

CREATE TABLE IF NOT EXISTS public.workforce_members (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  first_name character varying(100) NOT NULL,
  last_name character varying(100) NOT NULL,
  email character varying(255),
  phone character varying(50),
  department character varying(120) NOT NULL,
  source_channel character varying(120) NOT NULL DEFAULT 'frontend:web:workforce',
  status character varying(20) DEFAULT 'pending'::character varying NOT NULL,
  notes text,
  birthday_month smallint CHECK (birthday_month BETWEEN 1 AND 12),
  birthday_day smallint CHECK (birthday_day BETWEEN 1 AND 31),
  created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  CONSTRAINT workforce_members_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS public.members (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  first_name varchar(100) NOT NULL,
  last_name varchar(100) NOT NULL,
  email varchar(255) NOT NULL,
  phone varchar(50),
  is_active boolean NOT NULL DEFAULT true,
  birthday_month smallint CHECK (birthday_month BETWEEN 1 AND 12),
  birthday_day smallint CHECK (birthday_day BETWEEN 1 AND 31),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS public.leadership_members (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  first_name character varying(100) NOT NULL,
  last_name character varying(100) NOT NULL,
  email character varying(255),
  phone character varying(50),
  role character varying(30) NOT NULL,
  status character varying(40) DEFAULT 'pending'::character varying NOT NULL,
  bio text,
  image_url text,
  birthday_month smallint CHECK (birthday_month BETWEEN 1 AND 12),
  birthday_day smallint CHECK (birthday_day BETWEEN 1 AND 31),
  anniversary_month smallint CHECK (anniversary_month BETWEEN 1 AND 12),
  anniversary_day smallint CHECK (anniversary_day BETWEEN 1 AND 31),
  created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  CONSTRAINT leadership_members_pkey PRIMARY KEY (id),
  CONSTRAINT leadership_members_role_check
    CHECK (role IN ('senior_pastor', 'associate_pastor', 'deacon', 'deaconess', 'reverend')),
  CONSTRAINT leadership_members_status_check
    CHECK (status IN ('pending', 'awaiting_super_admin_approval', 'approved', 'declined'))
);

-- =========================
-- FORMS
-- =========================

CREATE TABLE IF NOT EXISTS public.forms (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  title character varying(255) NOT NULL,
  description text,
  event_id uuid,
  slug character varying(255),
  is_published boolean DEFAULT false NOT NULL,
  status character varying(20) NOT NULL DEFAULT 'draft',
  report_access_token character varying(160),
  published_at timestamp with time zone,
  settings jsonb,
  created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  deleted_at timestamp with time zone,
  CONSTRAINT forms_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS public.form_fields (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  form_id uuid NOT NULL,
  key character varying(100) NOT NULL,
  label character varying(255) NOT NULL,
  type character varying(30) NOT NULL,
  required boolean DEFAULT false NOT NULL,
  options jsonb,
  validation jsonb,
  visibility jsonb,
  "order" bigint DEFAULT 0 NOT NULL,
  created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  deleted_at timestamp with time zone,
  CONSTRAINT form_fields_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS public.form_submissions (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  form_id uuid NOT NULL,
  "values" jsonb NOT NULL,
  name character varying(255),
  email character varying(255),
  contact_number character varying(100),
  contact_address character varying(500),
  registration_code character varying(40),
  created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  deleted_at timestamp with time zone,
  CONSTRAINT form_submissions_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS public.form_calendar_reminders (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  form_id uuid NOT NULL,
  submission_id uuid NOT NULL,
  slug character varying(255) NOT NULL,
  email character varying(255) NOT NULL,
  recipient_name character varying(255),
  registration_code character varying(64),
  event_title character varying(255) NOT NULL,
  event_location character varying(255),
  event_date character varying(20) NOT NULL,
  event_time character varying(64) NOT NULL,
  event_starts_at timestamp with time zone NOT NULL,
  event_ends_at timestamp with time zone,
  calendar_token character varying(120) NOT NULL,
  opted_in_at timestamp with time zone,
  reminder_sent_at timestamp with time zone,
  created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  CONSTRAINT form_calendar_reminders_pkey PRIMARY KEY (id)
);

-- =========================
-- ASSETS / EMAIL TEMPLATES
-- =========================

CREATE TABLE IF NOT EXISTS public.assets (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  owner_type character varying(50),
  owner_id uuid,
  kind character varying(50),
  provider character varying(50) NOT NULL,
  bucket character varying(255) NOT NULL,
  object_key text NOT NULL,
  public_url text NOT NULL,
  content_type character varying(255),
  size_bytes bigint,
  checksum character varying(128),
  status character varying(20) NOT NULL DEFAULT 'pending',
  metadata jsonb,
  created_by_id uuid,
  created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  deleted_at timestamp with time zone,
  CONSTRAINT assets_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS public.email_templates (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  template_key character varying(255) NOT NULL,
  owner_type character varying(50),
  owner_id uuid,
  subject character varying(255),
  html_body text NOT NULL,
  text_body text,
  status character varying(20) NOT NULL DEFAULT 'draft',
  version integer NOT NULL DEFAULT 1,
  is_active boolean NOT NULL DEFAULT false,
  metadata jsonb,
  created_by_id uuid,
  created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  deleted_at timestamp with time zone,
  CONSTRAINT email_templates_pkey PRIMARY KEY (id)
);

-- =========================
-- ENSURE LATE-ADDED COLUMNS (IDEMPOTENT)
-- =========================

ALTER TABLE public.events
  ADD COLUMN IF NOT EXISTS image_key text,
  ADD COLUMN IF NOT EXISTS banner_image_key text;

ALTER TABLE public.forms
  ADD COLUMN IF NOT EXISTS status character varying(20) NOT NULL DEFAULT 'draft',
  ADD COLUMN IF NOT EXISTS report_access_token character varying(160),
  ADD COLUMN IF NOT EXISTS published_at timestamp with time zone;

ALTER TABLE public.users
  ADD COLUMN IF NOT EXISTS federated_provider character varying(50),
  ADD COLUMN IF NOT EXISTS federated_subject character varying(255),
  ADD COLUMN IF NOT EXISTS federated_linked_at timestamp with time zone,
  ADD COLUMN IF NOT EXISTS preferred_mfa_method character varying(30) NOT NULL DEFAULT 'email_otp',
  ADD COLUMN IF NOT EXISTS totp_enabled boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS totp_secret_enc text,
  ADD COLUMN IF NOT EXISTS totp_pending_enc text;

ALTER TABLE public.form_fields
  ADD COLUMN IF NOT EXISTS validation jsonb,
  ADD COLUMN IF NOT EXISTS visibility jsonb;

ALTER TABLE public.form_submissions
  ADD COLUMN IF NOT EXISTS registration_code character varying(40);

ALTER TABLE public.workforce_members
  ADD COLUMN IF NOT EXISTS birthday_month smallint CHECK (birthday_month BETWEEN 1 AND 12),
  ADD COLUMN IF NOT EXISTS birthday_day smallint CHECK (birthday_day BETWEEN 1 AND 31);

ALTER TABLE public.members
  ADD COLUMN IF NOT EXISTS birthday_month smallint CHECK (birthday_month BETWEEN 1 AND 12),
  ADD COLUMN IF NOT EXISTS birthday_day smallint CHECK (birthday_day BETWEEN 1 AND 31);

ALTER TABLE public.leadership_members
  ADD COLUMN IF NOT EXISTS birthday_month smallint CHECK (birthday_month BETWEEN 1 AND 12),
  ADD COLUMN IF NOT EXISTS birthday_day smallint CHECK (birthday_day BETWEEN 1 AND 31),
  ADD COLUMN IF NOT EXISTS anniversary_month smallint CHECK (anniversary_month BETWEEN 1 AND 12),
  ADD COLUMN IF NOT EXISTS anniversary_day smallint CHECK (anniversary_day BETWEEN 1 AND 31);

-- =========================
-- TYPE ALIGNMENT / DATA NORMALIZATION
-- =========================

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'public'
      AND table_name = 'events'
      AND column_name = 'date'
      AND data_type <> 'character varying'
  ) THEN
    ALTER TABLE public.events
      ALTER COLUMN "date" TYPE varchar(20) USING "date"::text;
  END IF;

  IF EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'public'
      AND table_name = 'events'
      AND column_name = 'time'
      AND data_type <> 'character varying'
  ) THEN
    ALTER TABLE public.events
      ALTER COLUMN "time" TYPE varchar(50) USING "time"::text;
  END IF;
END $$;

UPDATE public.events
SET status = CASE
    WHEN "date"::text ~ '^\\d{4}-\\d{2}-\\d{2}$' AND to_date("date"::text, 'YYYY-MM-DD') = CURRENT_DATE THEN 'happening'
    WHEN "date"::text ~ '^\\d{4}-\\d{2}-\\d{2}$' AND to_date("date"::text, 'YYYY-MM-DD') > CURRENT_DATE THEN 'upcoming'
    WHEN status IN ('upcoming', 'happening', 'past') THEN status
    ELSE 'past'
  END
WHERE status IS NULL OR status NOT IN ('upcoming', 'happening', 'past');

ALTER TABLE public.events
  ALTER COLUMN status SET DEFAULT 'upcoming';

ALTER TABLE public.events
  DROP CONSTRAINT IF EXISTS events_status_check;

ALTER TABLE public.events
  ADD CONSTRAINT events_status_check
  CHECK (status IN ('upcoming', 'happening', 'past'));

UPDATE public.forms
SET status = CASE
    WHEN status IN ('draft', 'published', 'invalid') THEN status
    WHEN is_published THEN 'published'
    ELSE 'draft'
  END
WHERE status IS NULL OR status = '' OR status NOT IN ('draft', 'published', 'invalid');

UPDATE public.forms
SET published_at = COALESCE(published_at, updated_at, created_at)
WHERE is_published = true AND published_at IS NULL;

ALTER TABLE public.forms
  ALTER COLUMN status SET DEFAULT 'draft';

ALTER TABLE public.forms
  DROP CONSTRAINT IF EXISTS forms_status_check;

ALTER TABLE public.forms
  ADD CONSTRAINT forms_status_check
  CHECK (status IN ('draft', 'published', 'invalid'));

UPDATE public.users
SET preferred_mfa_method = CASE
    WHEN preferred_mfa_method IN ('email_otp', 'totp') THEN preferred_mfa_method
    ELSE 'email_otp'
  END
WHERE preferred_mfa_method IS NULL OR preferred_mfa_method NOT IN ('email_otp', 'totp');

ALTER TABLE public.users
  ALTER COLUMN preferred_mfa_method SET DEFAULT 'email_otp';

UPDATE public.leadership_members
SET role = CASE
    WHEN role = 'senior_pastor' THEN role
    WHEN role IN ('associate_pastor', 'deacon', 'deaconess', 'reverend') THEN role
    ELSE 'associate_pastor'
  END
WHERE role IS NULL OR role NOT IN ('senior_pastor', 'associate_pastor', 'deacon', 'deaconess', 'reverend');

UPDATE public.leadership_members
SET status = CASE
    WHEN status IN ('pending', 'awaiting_super_admin_approval', 'approved', 'declined') THEN status
    ELSE 'pending'
  END
WHERE status IS NULL OR status NOT IN ('pending', 'awaiting_super_admin_approval', 'approved', 'declined');

ALTER TABLE public.leadership_members
  DROP CONSTRAINT IF EXISTS leadership_members_role_check;

ALTER TABLE public.leadership_members
  ADD CONSTRAINT leadership_members_role_check
  CHECK (role IN ('senior_pastor', 'associate_pastor', 'deacon', 'deaconess', 'reverend'));

ALTER TABLE public.leadership_members
  DROP CONSTRAINT IF EXISTS leadership_members_status_check;

ALTER TABLE public.leadership_members
  ADD CONSTRAINT leadership_members_status_check
  CHECK (status IN ('pending', 'awaiting_super_admin_approval', 'approved', 'declined'));

-- =========================
-- FOREIGN KEYS (IDEMPOTENT)
-- =========================

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_trusted_devices_user_id') THEN
    ALTER TABLE public.trusted_devices
      ADD CONSTRAINT fk_trusted_devices_user_id
      FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_security_events_user_id') THEN
    ALTER TABLE public.security_events
      ADD CONSTRAINT fk_security_events_user_id
      FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE SET NULL;
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_reels_event_id') THEN
    ALTER TABLE public.reels
      ADD CONSTRAINT fk_reels_event_id
      FOREIGN KEY (event_id) REFERENCES public.events(id) ON DELETE SET NULL;
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_notifications_event_id') THEN
    ALTER TABLE public.notifications
      ADD CONSTRAINT fk_notifications_event_id
      FOREIGN KEY (event_id) REFERENCES public.events(id) ON DELETE SET NULL;
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_notification_deliveries_notification_id') THEN
    ALTER TABLE public.notification_deliveries
      ADD CONSTRAINT fk_notification_deliveries_notification_id
      FOREIGN KEY (notification_id) REFERENCES public.notifications(id) ON DELETE CASCADE;
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_notification_deliveries_subscriber_id') THEN
    ALTER TABLE public.notification_deliveries
      ADD CONSTRAINT fk_notification_deliveries_subscriber_id
      FOREIGN KEY (subscriber_id) REFERENCES public.subscribers(id) ON DELETE CASCADE;
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_forms_event_id') THEN
    ALTER TABLE public.forms
      ADD CONSTRAINT fk_forms_event_id
      FOREIGN KEY (event_id) REFERENCES public.events(id) ON DELETE SET NULL;
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_form_fields_form_id') THEN
    ALTER TABLE public.form_fields
      ADD CONSTRAINT fk_form_fields_form_id
      FOREIGN KEY (form_id) REFERENCES public.forms(id) ON DELETE CASCADE;
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_form_submissions_form_id') THEN
    ALTER TABLE public.form_submissions
      ADD CONSTRAINT fk_form_submissions_form_id
      FOREIGN KEY (form_id) REFERENCES public.forms(id) ON DELETE CASCADE;
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_form_calendar_reminders_form_id') THEN
    ALTER TABLE public.form_calendar_reminders
      ADD CONSTRAINT fk_form_calendar_reminders_form_id
      FOREIGN KEY (form_id) REFERENCES public.forms(id) ON DELETE CASCADE;
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_form_calendar_reminders_submission_id') THEN
    ALTER TABLE public.form_calendar_reminders
      ADD CONSTRAINT fk_form_calendar_reminders_submission_id
      FOREIGN KEY (submission_id) REFERENCES public.form_submissions(id) ON DELETE CASCADE;
  END IF;
END $$;

-- =========================
-- PEOPLE DOMAIN SAFETY CLEANUP
-- =========================

ALTER TABLE IF EXISTS public.members
  DROP CONSTRAINT IF EXISTS uni_members_email;

ALTER TABLE IF EXISTS public.members
  DROP CONSTRAINT IF EXISTS members_email_key;

ALTER TABLE IF EXISTS public.workforce_members
  DROP CONSTRAINT IF EXISTS uni_workforce_members_email;

ALTER TABLE IF EXISTS public.leadership_members
  DROP CONSTRAINT IF EXISTS uni_leadership_members_email;

DROP INDEX IF EXISTS public.uni_members_email;
DROP INDEX IF EXISTS public.idx_members_email_unique;
DROP INDEX IF EXISTS public.uni_workforce_members_email;
DROP INDEX IF EXISTS public.idx_workforce_members_email_unique;
DROP INDEX IF EXISTS public.uni_leadership_members_email;
DROP INDEX IF EXISTS public.idx_leadership_members_email_unique;

ALTER TABLE IF EXISTS public.leadership_members
  ALTER COLUMN status TYPE varchar(40) USING status::text;

UPDATE public.leadership_members
SET status = CASE
    WHEN status IN ('pending', 'awaiting_super_admin_approval', 'approved', 'declined') THEN status
    ELSE 'pending'
  END
WHERE status IS NULL OR status NOT IN ('pending', 'awaiting_super_admin_approval', 'approved', 'declined');

ALTER TABLE IF EXISTS public.leadership_members
  DROP CONSTRAINT IF EXISTS leadership_members_status_check;

ALTER TABLE IF EXISTS public.leadership_members
  ADD CONSTRAINT leadership_members_status_check
  CHECK (status IN ('pending', 'awaiting_super_admin_approval', 'approved', 'declined'));

-- =========================
-- INDEXES (IDEMPOTENT)
-- =========================

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_unique
  ON public.users (email)
  WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_federated_subject_unique
  ON public.users (federated_subject)
  WHERE federated_subject IS NOT NULL AND deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_subscribers_email_unique
  ON public.subscribers (email)
  WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_site_contents_key_unique
  ON public.site_contents (key);

CREATE UNIQUE INDEX IF NOT EXISTS idx_forms_slug_unique
  ON public.forms (slug)
  WHERE slug IS NOT NULL AND deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_forms_report_access_token_unique
  ON public.forms (report_access_token)
  WHERE report_access_token IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_trusted_devices_user_id
  ON public.trusted_devices (user_id);

CREATE INDEX IF NOT EXISTS idx_trusted_devices_device_id
  ON public.trusted_devices (device_id);

CREATE INDEX IF NOT EXISTS idx_security_events_user_id
  ON public.security_events (user_id);

CREATE INDEX IF NOT EXISTS idx_security_events_email
  ON public.security_events (email);

CREATE INDEX IF NOT EXISTS idx_otps_email
  ON public.otps (email);

CREATE INDEX IF NOT EXISTS idx_otps_purpose
  ON public.otps (purpose);

CREATE INDEX IF NOT EXISTS idx_events_status
  ON public.events (status);

CREATE INDEX IF NOT EXISTS idx_events_date
  ON public.events (date);

CREATE INDEX IF NOT EXISTS idx_reels_event_id
  ON public.reels (event_id);

CREATE INDEX IF NOT EXISTS idx_notifications_event_id
  ON public.notifications (event_id);

CREATE INDEX IF NOT EXISTS idx_notification_deliveries_notification_id
  ON public.notification_deliveries (notification_id);

CREATE INDEX IF NOT EXISTS idx_notification_deliveries_subscriber_id
  ON public.notification_deliveries (subscriber_id);

CREATE INDEX IF NOT EXISTS idx_workforce_bday_month_day
  ON public.workforce_members (birthday_month, birthday_day);

CREATE INDEX IF NOT EXISTS idx_workforce_source_channel
  ON public.workforce_members (source_channel);


CREATE INDEX IF NOT EXISTS idx_members_email
  ON public.members (email);

CREATE INDEX IF NOT EXISTS idx_workforce_members_email
  ON public.workforce_members (email);

CREATE INDEX IF NOT EXISTS idx_members_birthday_month_day
  ON public.members (birthday_month, birthday_day);

CREATE INDEX IF NOT EXISTS idx_leadership_role_status
  ON public.leadership_members (role, status);

CREATE INDEX IF NOT EXISTS idx_leadership_status
  ON public.leadership_members (status);

CREATE INDEX IF NOT EXISTS idx_leadership_email
  ON public.leadership_members (email);

CREATE INDEX IF NOT EXISTS idx_leadership_anniversary_month_day
  ON public.leadership_members (anniversary_month, anniversary_day);

CREATE INDEX IF NOT EXISTS idx_pastoral_care_requests_created_at
  ON public.pastoral_care_requests (created_at);

CREATE INDEX IF NOT EXISTS idx_giving_intents_created_at
  ON public.giving_intents (created_at);

CREATE INDEX IF NOT EXISTS idx_forms_event_id
  ON public.forms (event_id);

CREATE INDEX IF NOT EXISTS idx_forms_status
  ON public.forms (status, is_published);

CREATE INDEX IF NOT EXISTS idx_form_fields_form_id
  ON public.form_fields (form_id);

CREATE INDEX IF NOT EXISTS idx_form_fields_form_id_order
  ON public.form_fields (form_id, "order");

CREATE INDEX IF NOT EXISTS idx_form_submissions_form_id
  ON public.form_submissions (form_id);

CREATE INDEX IF NOT EXISTS idx_form_submissions_form_id_created_at
  ON public.form_submissions (form_id, created_at);

CREATE UNIQUE INDEX IF NOT EXISTS idx_form_calendar_reminders_submission_id
  ON public.form_calendar_reminders (submission_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_form_calendar_reminders_token
  ON public.form_calendar_reminders (calendar_token);

CREATE INDEX IF NOT EXISTS idx_form_calendar_reminders_slug
  ON public.form_calendar_reminders (slug);

CREATE INDEX IF NOT EXISTS idx_form_calendar_reminders_email
  ON public.form_calendar_reminders (email);

CREATE INDEX IF NOT EXISTS idx_form_calendar_reminders_due
  ON public.form_calendar_reminders (opted_in_at, reminder_sent_at, event_starts_at);

CREATE UNIQUE INDEX IF NOT EXISTS idx_assets_object_key_unique
  ON public.assets (object_key)
  WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_assets_owner
  ON public.assets (owner_type, owner_id);

CREATE INDEX IF NOT EXISTS idx_assets_kind
  ON public.assets (kind);

CREATE INDEX IF NOT EXISTS idx_assets_status
  ON public.assets (status);

CREATE INDEX IF NOT EXISTS idx_email_templates_owner
  ON public.email_templates (owner_type, owner_id);

CREATE INDEX IF NOT EXISTS idx_email_templates_key
  ON public.email_templates (template_key);

CREATE UNIQUE INDEX IF NOT EXISTS idx_email_templates_key_version_unique
  ON public.email_templates (template_key, version)
  WHERE deleted_at IS NULL;

-- migration: 011_consolidated_incremental_schema.up.sql
-- Consolidated incremental schema migration.
--
-- Supersedes the ten migration files that used to live here:
--   001_consolidated_incremental_schema (itself a prior consolidation of
--     account lockout, refresh tokens, campuses, giving, attendance, cell
--     groups, prayer requests, performance indexes, and ministries)
--   002_audit_logs
--   003_schema_drift_reconciliation
--   004_approval_request_reason
--   005_workforce_anniversary
--   006_prayer_request_integrity
--   007_analytics_pipeline
--   008_new_member_workflows
--   009_normalize_ministry_workforce
--   010_backfill_workforce_dates
-- Their content is merged below, unchanged, in the same order they were
-- originally applied (dependencies between them — e.g. 004 altering a table
-- 003 creates — require that order to be preserved).
--
-- Every statement here is idempotent (IF NOT EXISTS / ON CONFLICT DO NOTHING /
-- guarded ADD CONSTRAINT, or backfills keyed on natural idempotency such as
-- COALESCE-only UPDATEs), so this file is safe to run against a database
-- regardless of whether it already has none, some, or all of the ten former
-- files applied — it converges every environment to the same end state
-- without re-doing or losing anything already there.

-- =============================================================================
-- Formerly 001_consolidated_incremental_schema
-- =============================================================================

-- ---------------------------------------------------------------------------
-- Formerly 001/001_add_account_lockout: account lockout tracking on users
-- ---------------------------------------------------------------------------
ALTER TABLE users
ADD COLUMN IF NOT EXISTS failed_login_count INTEGER DEFAULT 0,
ADD COLUMN IF NOT EXISTS last_failed_login_at TIMESTAMP NULL,
ADD COLUMN IF NOT EXISTS is_locked BOOLEAN DEFAULT FALSE,
ADD COLUMN IF NOT EXISTS locked_until TIMESTAMP NULL;

CREATE INDEX IF NOT EXISTS idx_users_is_locked ON users(is_locked) WHERE is_locked = true;
CREATE INDEX IF NOT EXISTS idx_users_locked_until ON users(locked_until) WHERE locked_until IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Formerly 001/002_add_trusted_devices_constraint: unique (user_id, device_id)
-- ---------------------------------------------------------------------------
DELETE FROM trusted_devices t1
WHERE EXISTS (
  SELECT 1 FROM trusted_devices t2
  WHERE t1.user_id = t2.user_id
  AND t1.device_id = t2.device_id
  AND t1.id > t2.id
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'uq_trusted_devices_user_device'
    ) THEN
        ALTER TABLE trusted_devices
        ADD CONSTRAINT uq_trusted_devices_user_device
        UNIQUE (user_id, device_id);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_trusted_devices_user_id ON trusted_devices(user_id);
CREATE INDEX IF NOT EXISTS idx_trusted_devices_expires_at ON trusted_devices(expires_at) WHERE deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- Formerly 001/003_add_refresh_tokens: refresh token family/rotation support
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS refresh_tokens (
    id          UUID        DEFAULT gen_random_uuid() PRIMARY KEY,
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    family_id   UUID        NOT NULL,
    token_hash  CHAR(64)    NOT NULL,          -- SHA-256 hex of raw token
    device_id   VARCHAR(255),
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    CONSTRAINT  refresh_tokens_hash_unique UNIQUE (token_hash)
);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_family
    ON refresh_tokens(family_id);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user
    ON refresh_tokens(user_id);

-- Background cleanup of expired, non-revoked tokens.
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_cleanup
    ON refresh_tokens(expires_at)
    WHERE revoked_at IS NULL;

-- ---------------------------------------------------------------------------
-- Formerly 001/004_encrypt_member_pii: encrypted phone column on members
-- ---------------------------------------------------------------------------
ALTER TABLE members ADD COLUMN IF NOT EXISTS phone_enc TEXT;

-- ---------------------------------------------------------------------------
-- Formerly 001/005_campus: multi-campus support
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS campuses (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    address     TEXT NOT NULL DEFAULT '',
    city        TEXT NOT NULL DEFAULT '',
    phone_enc   TEXT,
    time_zone   TEXT NOT NULL DEFAULT 'UTC',
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ,
    CONSTRAINT campuses_name_unique UNIQUE (name)
);

ALTER TABLE members           ADD COLUMN IF NOT EXISTS campus_id UUID REFERENCES campuses(id) ON DELETE SET NULL;
ALTER TABLE events            ADD COLUMN IF NOT EXISTS campus_id UUID REFERENCES campuses(id) ON DELETE SET NULL;
ALTER TABLE workforce_members ADD COLUMN IF NOT EXISTS campus_id UUID REFERENCES campuses(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_campuses_active ON campuses(is_active) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_members_campus  ON members(campus_id) WHERE campus_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_events_campus   ON events(campus_id)  WHERE campus_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Formerly 001/006_giving_transactions: giving / financial transactions
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS giving_categories (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    code       TEXT NOT NULL,
    is_active  BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT giving_categories_name_unique UNIQUE (name),
    CONSTRAINT giving_categories_code_unique UNIQUE (code)
);

INSERT INTO giving_categories (name, code) VALUES
    ('Tithe',          'tithe'),
    ('Offering',       'offering'),
    ('Building Fund',  'building_fund'),
    ('Welfare',        'welfare'),
    ('Missions',       'missions')
ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS giving_transactions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    category_id      UUID NOT NULL REFERENCES giving_categories(id),
    member_id        UUID REFERENCES members(id) ON DELETE SET NULL,
    campus_id        UUID REFERENCES campuses(id) ON DELETE SET NULL,
    amount_kobo      BIGINT NOT NULL CHECK (amount_kobo > 0),
    currency         CHAR(3) NOT NULL DEFAULT 'NGN',
    channel          TEXT NOT NULL,           -- card, transfer, cash, ussd
    payment_ref      TEXT UNIQUE,
    payment_provider TEXT,                    -- paystack, stripe, manual
    status           TEXT NOT NULL DEFAULT 'pending',  -- pending, success, failed, reversed
    giver_name       TEXT NOT NULL DEFAULT '',
    giver_email      TEXT NOT NULL DEFAULT '',
    recorded_by_id   UUID REFERENCES users(id) ON DELETE SET NULL,
    given_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_giving_category_date
    ON giving_transactions(category_id, given_at DESC) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_giving_member
    ON giving_transactions(member_id, given_at DESC)
    WHERE member_id IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_giving_status
    ON giving_transactions(status, given_at DESC) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_giving_campus
    ON giving_transactions(campus_id, given_at DESC)
    WHERE campus_id IS NOT NULL AND deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- Formerly 001/007_attendance: attendance tracking
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS service_types (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    campus_id  UUID REFERENCES campuses(id) ON DELETE SET NULL,
    is_active  BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

INSERT INTO service_types (name) VALUES
    ('Sunday First Service'),
    ('Sunday Second Service'),
    ('Wednesday Service'),
    ('Friday Vigil')
ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS attendance_sessions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campus_id       UUID REFERENCES campuses(id) ON DELETE SET NULL,
    service_type_id UUID NOT NULL REFERENCES service_types(id),
    date            DATE NOT NULL,
    head_count      INT NOT NULL DEFAULT 0 CHECK (head_count >= 0),
    notes           TEXT NOT NULL DEFAULT '',
    created_by_id   UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS attendance_records (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id     UUID NOT NULL REFERENCES attendance_sessions(id) ON DELETE CASCADE,
    member_id      UUID REFERENCES members(id) ON DELETE SET NULL,
    guest_name     TEXT NOT NULL DEFAULT '',
    checked_in_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    checked_in_via TEXT NOT NULL DEFAULT 'manual',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at     TIMESTAMPTZ,
    -- A member can only be checked in once per session.
    CONSTRAINT attendance_records_session_member_unique UNIQUE (session_id, member_id)
);

CREATE INDEX IF NOT EXISTS idx_attendance_sessions_date
    ON attendance_sessions(date DESC) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_attendance_sessions_campus_date
    ON attendance_sessions(campus_id, date DESC)
    WHERE campus_id IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_attendance_records_session
    ON attendance_records(session_id) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_attendance_records_member
    ON attendance_records(member_id, checked_in_at DESC)
    WHERE member_id IS NOT NULL AND deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- Formerly 001/008_cell_groups: cell groups / small groups management
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS cell_groups (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL,
    campus_id    UUID REFERENCES campuses(id) ON DELETE SET NULL,
    leader_id    UUID REFERENCES members(id) ON DELETE SET NULL,
    zone         TEXT NOT NULL DEFAULT '',
    max_capacity INT NOT NULL DEFAULT 0 CHECK (max_capacity >= 0),
    is_active    BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at   TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS cell_group_members (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id   UUID NOT NULL REFERENCES cell_groups(id) ON DELETE CASCADE,
    member_id  UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    role       TEXT NOT NULL DEFAULT 'member',
    joined_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT cell_group_members_unique UNIQUE (group_id, member_id)
);

CREATE TABLE IF NOT EXISTS cell_group_meetings (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id       UUID NOT NULL REFERENCES cell_groups(id) ON DELETE CASCADE,
    date           TIMESTAMPTZ NOT NULL,
    attendee_count INT NOT NULL DEFAULT 0 CHECK (attendee_count >= 0),
    notes          TEXT NOT NULL DEFAULT '',
    led_by_id      UUID REFERENCES members(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_cell_groups_campus    ON cell_groups(campus_id) WHERE campus_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_cell_group_members_m  ON cell_group_members(member_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_cell_group_meetings_g ON cell_group_meetings(group_id, date DESC) WHERE deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- Formerly 001/009_prayer_requests
-- ---------------------------------------------------------------------------
-- All body content is AES-256-GCM encrypted at the application layer.
CREATE TABLE IF NOT EXISTS prayer_requests (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    member_id    UUID REFERENCES members(id) ON DELETE SET NULL,
    first_name   TEXT NOT NULL DEFAULT '',
    last_name    TEXT NOT NULL DEFAULT '',
    email        TEXT NOT NULL DEFAULT '',
    request_enc  TEXT NOT NULL,        -- AES-256-GCM ciphertext, never plaintext
    category     TEXT NOT NULL DEFAULT '',
    is_anonymous BOOLEAN NOT NULL DEFAULT FALSE,
    status       TEXT NOT NULL DEFAULT 'pending',  -- pending, praying, answered, closed
    assigned_to  UUID REFERENCES users(id) ON DELETE SET NULL,
    notes_enc    TEXT,                 -- AES-256-GCM encrypted pastoral notes
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_prayer_requests_status
    ON prayer_requests(status, created_at DESC) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_prayer_requests_member
    ON prayer_requests(member_id) WHERE member_id IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_prayer_requests_assigned
    ON prayer_requests(assigned_to) WHERE assigned_to IS NOT NULL AND deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- Formerly 001/010_performance_indexes: performance indexes for hot query paths
-- (idx_refresh_tokens_cleanup omitted here — already defined above, was a
-- duplicate of the one created alongside refresh_tokens itself)
-- ---------------------------------------------------------------------------
CREATE INDEX IF NOT EXISTS idx_security_events_user_type
    ON security_events(user_id, type, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_otps_email_purpose_active
    ON otps(email, purpose, expires_at DESC)
    WHERE used_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_members_fts
    ON members USING GIN (to_tsvector('english', first_name || ' ' || last_name));

CREATE INDEX IF NOT EXISTS idx_members_birthday
    ON members(birthday_month, birthday_day)
    WHERE is_active = true AND birthday_month IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_trusted_devices_user_device
    ON trusted_devices(user_id, device_id, expires_at DESC)
    WHERE deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- Formerly 001/011_ministries: ministry / department management
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ministries (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    campus_id   UUID REFERENCES campuses(id) ON DELETE SET NULL,
    leader_id   UUID REFERENCES members(id) ON DELETE SET NULL,
    category    TEXT NOT NULL DEFAULT '',
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);

INSERT INTO ministries (name, category) VALUES
    ('Worship Team',        'worship'),
    ('Children Ministry',   'children'),
    ('Media & Technology',  'media'),
    ('Ushering',            'ushering'),
    ('Prayer Team',         'prayer'),
    ('Welfare',             'welfare')
ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS ministry_members (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ministry_id UUID NOT NULL REFERENCES ministries(id) ON DELETE CASCADE,
    member_id   UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    role        TEXT NOT NULL DEFAULT 'member',
    joined_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ,
    CONSTRAINT ministry_members_unique UNIQUE (ministry_id, member_id)
);

CREATE INDEX IF NOT EXISTS idx_ministries_campus   ON ministries(campus_id) WHERE campus_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_ministry_members_m  ON ministry_members(member_id) WHERE deleted_at IS NULL;

-- =============================================================================
-- Formerly 002_audit_logs
--
-- Durable audit log storage. Previously, admin/auth mutating requests were
-- only ever written to structured application logs (internal/logger), never
-- to the database — so the "recent activity" dashboard widget and the
-- /admin/audit-logs endpoint had nothing real to show and returned hardcoded
-- empty results. This table gives them something to query.
-- =============================================================================
CREATE TABLE IF NOT EXISTS audit_logs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scope       TEXT NOT NULL,
    method      TEXT NOT NULL,
    path        TEXT NOT NULL,
    status_code INT NOT NULL,
    latency_ms  BIGINT NOT NULL,
    user_id     UUID REFERENCES users(id) ON DELETE SET NULL,
    role        TEXT NOT NULL DEFAULT '',
    ip          TEXT NOT NULL DEFAULT '',
    user_agent  TEXT NOT NULL DEFAULT '',
    request_id  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at
    ON audit_logs(created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_logs_scope_created_at
    ON audit_logs(scope, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id
    ON audit_logs(user_id) WHERE user_id IS NOT NULL;

-- =============================================================================
-- Formerly 003_schema_drift_reconciliation
--
-- Reconciles tables/columns that exist only in Go models + conditional GORM
-- AutoMigrate (gated behind RUN_AUTOMIGRATE / ENSURE_ADMIN_SCHEMA_ON_STARTUP in
-- production, see internal/database/postgre.go) but were never captured in the
-- version-controlled raw SQL schema. Any environment provisioned from
-- migrations/*.sql alone (fresh deploy, staging, disaster recovery) was missing
-- these entirely. All statements are idempotent and additive only.
-- =============================================================================

-- EVENTS: approval workflow columns (internal/models/event.go)
ALTER TABLE events ADD COLUMN IF NOT EXISTS is_approved boolean NOT NULL DEFAULT false;
ALTER TABLE events ADD COLUMN IF NOT EXISTS approved_by_id uuid;
ALTER TABLE events ADD COLUMN IF NOT EXISTS approved_by_name varchar(120);
ALTER TABLE events ADD COLUMN IF NOT EXISTS approved_by_email varchar(255);
ALTER TABLE events ADD COLUMN IF NOT EXISTS approved_at timestamptz;

-- TESTIMONIALS: approval attribution columns (internal/models/testimonials.go)
ALTER TABLE testimonials ADD COLUMN IF NOT EXISTS approved_by_id uuid;
ALTER TABLE testimonials ADD COLUMN IF NOT EXISTS approved_by_name varchar(120);
ALTER TABLE testimonials ADD COLUMN IF NOT EXISTS approved_by_email varchar(255);
ALTER TABLE testimonials ADD COLUMN IF NOT EXISTS approved_at timestamptz;

-- ADMIN_NOTIFICATIONS (internal/models/admin_notification.go)
CREATE TABLE IF NOT EXISTS admin_notifications (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL,
    type        varchar(40) NOT NULL,
    title       varchar(255) NOT NULL,
    message     text NOT NULL,
    ticket_code varchar(50),
    entity_type varchar(40),
    entity_id   uuid,
    is_read     boolean NOT NULL DEFAULT false,
    read_at     timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_admin_notifications_user_id ON admin_notifications(user_id);

-- APPROVAL_REQUESTS (internal/models/approval_request.go)
CREATE TABLE IF NOT EXISTS approval_requests (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_code        varchar(50) NOT NULL,
    type               varchar(30) NOT NULL,
    status             varchar(20) NOT NULL DEFAULT 'pending',
    entity_id          uuid,
    entity_label       varchar(255),
    requested_by_id    uuid,
    requested_by_name  varchar(120),
    requested_by_email varchar(255),
    approved_by_id     uuid,
    approved_by_name   varchar(120),
    approved_by_email  varchar(255),
    approved_at        timestamptz,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_approval_requests_ticket_code ON approval_requests(ticket_code);

-- FORM_CAMPAIGN_DELIVERIES (internal/models/form_campaign_delivery.go)
CREATE TABLE IF NOT EXISTS form_campaign_deliveries (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    form_id            uuid NOT NULL,
    form_title         varchar(255) NOT NULL,
    event_id           uuid,
    event_title        varchar(255),
    subject            varchar(255) NOT NULL,
    template_source    varchar(120) NOT NULL,
    template_id        varchar(120),
    template_key       varchar(255),
    status             varchar(20) NOT NULL DEFAULT 'completed',
    total_recipients   int NOT NULL DEFAULT 0,
    targeted           int NOT NULL DEFAULT 0,
    sent               int NOT NULL DEFAULT 0,
    skipped            int NOT NULL DEFAULT 0,
    failed             int NOT NULL DEFAULT 0,
    failed_recipients  jsonb,
    started_at         timestamptz NOT NULL,
    completed_at       timestamptz,
    created_by_user_id uuid,
    created_by_email   varchar(255),
    created_by_role    varchar(50),
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    deleted_at         timestamptz
);
CREATE INDEX IF NOT EXISTS idx_form_campaign_deliveries_form_id      ON form_campaign_deliveries(form_id);
CREATE INDEX IF NOT EXISTS idx_form_campaign_deliveries_event_id     ON form_campaign_deliveries(event_id);
CREATE INDEX IF NOT EXISTS idx_form_campaign_deliveries_status       ON form_campaign_deliveries(status);
CREATE INDEX IF NOT EXISTS idx_form_campaign_deliveries_started_at   ON form_campaign_deliveries(started_at);
CREATE INDEX IF NOT EXISTS idx_form_campaign_deliveries_completed_at ON form_campaign_deliveries(completed_at);
CREATE INDEX IF NOT EXISTS idx_form_campaign_deliveries_deleted_at   ON form_campaign_deliveries(deleted_at);

-- ADMIN_EMAIL_DELIVERIES (internal/models/admin_email_delivery.go)
CREATE TABLE IF NOT EXISTS admin_email_deliveries (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    subject            varchar(255) NOT NULL,
    template_source    varchar(120) NOT NULL,
    template_id        varchar(120),
    template_key       varchar(255),
    audience_source    varchar(20) NOT NULL DEFAULT 'manual',
    manual_recipients  int NOT NULL DEFAULT 0,
    form_recipients    int NOT NULL DEFAULT 0,
    source_forms       jsonb,
    status             varchar(20) NOT NULL DEFAULT 'completed',
    total_recipients   int NOT NULL DEFAULT 0,
    targeted           int NOT NULL DEFAULT 0,
    sent               int NOT NULL DEFAULT 0,
    skipped            int NOT NULL DEFAULT 0,
    failed             int NOT NULL DEFAULT 0,
    failed_recipients  jsonb,
    started_at         timestamptz NOT NULL,
    completed_at       timestamptz,
    created_by_user_id uuid,
    created_by_email   varchar(255),
    created_by_role    varchar(50),
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    deleted_at         timestamptz
);
CREATE INDEX IF NOT EXISTS idx_admin_email_deliveries_audience_source ON admin_email_deliveries(audience_source);
CREATE INDEX IF NOT EXISTS idx_admin_email_deliveries_status          ON admin_email_deliveries(status);
CREATE INDEX IF NOT EXISTS idx_admin_email_deliveries_started_at      ON admin_email_deliveries(started_at);
CREATE INDEX IF NOT EXISTS idx_admin_email_deliveries_completed_at    ON admin_email_deliveries(completed_at);
CREATE INDEX IF NOT EXISTS idx_admin_email_deliveries_deleted_at      ON admin_email_deliveries(deleted_at);

-- REGISTRATION_SEQUENCES (internal/models/registration_sequence.go)
CREATE TABLE IF NOT EXISTS registration_sequences (
    prefix      varchar(20) PRIMARY KEY,
    last_number int NOT NULL DEFAULT 0,
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- TICKET_SEQUENCES (internal/models/ticket_sequence.go)
CREATE TABLE IF NOT EXISTS ticket_sequences (
    prefix      varchar(40) PRIMARY KEY,
    last_number int NOT NULL DEFAULT 0,
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- ANALYTICS_BATCHES (internal/models/analytics_batch.go)
CREATE TABLE IF NOT EXISTS analytics_batches (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id    varchar(120) NOT NULL,
    session_id  varchar(120) NOT NULL,
    user_id     varchar(120),
    event_count int NOT NULL DEFAULT 0,
    payload     jsonb NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_analytics_batches_batch_id   ON analytics_batches(batch_id);
CREATE INDEX IF NOT EXISTS idx_analytics_batches_session_id ON analytics_batches(session_id);
CREATE INDEX IF NOT EXISTS idx_analytics_batches_user_id    ON analytics_batches(user_id);

-- STORE_PRODUCTS / STORE_ORDERS / STORE_ORDER_ITEMS (internal/models/store.go)
CREATE TABLE IF NOT EXISTS store_products (
    id             bigserial PRIMARY KEY,
    name           varchar(200) NOT NULL,
    category       varchar(80) NOT NULL,
    price          varchar(40) NOT NULL,
    original_price varchar(40),
    image          text NOT NULL,
    description    text NOT NULL,
    sizes          jsonb NOT NULL DEFAULT '[]',
    colors         jsonb NOT NULL DEFAULT '[]',
    tags           jsonb NOT NULL DEFAULT '[]',
    stock          int NOT NULL DEFAULT 0,
    is_active      boolean NOT NULL DEFAULT true,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_store_products_category ON store_products(category);

CREATE TABLE IF NOT EXISTS store_orders (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id              varchar(120) NOT NULL,
    status                varchar(30) NOT NULL DEFAULT 'pending',
    subtotal              double precision NOT NULL DEFAULT 0,
    delivery_fee          double precision NOT NULL DEFAULT 0,
    total                 double precision NOT NULL DEFAULT 0,
    payment_method        varchar(40) NOT NULL,
    customer_first_name   varchar(120) NOT NULL,
    customer_last_name    varchar(120) NOT NULL,
    customer_email        varchar(255) NOT NULL,
    customer_phone        varchar(64) NOT NULL,
    customer_address      text,
    customer_city         varchar(120),
    customer_state        varchar(120),
    customer_zip_code     varchar(40),
    customer_account_name varchar(180),
    customer_bank_name    varchar(180),
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_store_orders_order_id ON store_orders(order_id);
CREATE INDEX IF NOT EXISTS idx_store_orders_customer_email ON store_orders(customer_email);

CREATE TABLE IF NOT EXISTS store_order_items (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    store_order_id uuid NOT NULL,
    product_id     bigint,
    name           varchar(220) NOT NULL,
    price          varchar(40) NOT NULL,
    quantity       int NOT NULL DEFAULT 1,
    selected_size  varchar(80) NOT NULL,
    selected_color varchar(80) NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'fk_store_order_items_store_order'
    ) THEN
        ALTER TABLE store_order_items
            ADD CONSTRAINT fk_store_order_items_store_order
            FOREIGN KEY (store_order_id) REFERENCES store_orders(id) ON DELETE CASCADE;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_store_order_items_store_order_id ON store_order_items(store_order_id);
CREATE INDEX IF NOT EXISTS idx_store_order_items_product_id     ON store_order_items(product_id);

-- =============================================================================
-- Formerly 004_approval_request_reason
--
-- Adds the stated reason a requester gives for an approval request — needed
-- for delete-approval flows (event/workforce/leadership deletion) so the
-- super-admin reviewing the request has actual context instead of just an
-- entity label. Nullable: existing rows and non-delete request types don't
-- require one.
-- =============================================================================
ALTER TABLE approval_requests ADD COLUMN IF NOT EXISTS reason TEXT;

-- =============================================================================
-- Formerly 005_workforce_anniversary
--
-- Adds wedding-anniversary tracking to workforce_members, mirroring the
-- birthday month/day columns already there and the anniversary columns
-- leadership_members already has. Without this, a workforce registration
-- form asking for an anniversary date had nowhere to store the answer.
-- =============================================================================
ALTER TABLE workforce_members
  ADD COLUMN IF NOT EXISTS anniversary_month smallint CHECK (anniversary_month BETWEEN 1 AND 12),
  ADD COLUMN IF NOT EXISTS anniversary_day smallint CHECK (anniversary_day BETWEEN 1 AND 31);

-- =============================================================================
-- Formerly 006_prayer_request_integrity
-- =============================================================================
UPDATE prayer_requests
SET status = 'pending'
WHERE status NOT IN ('pending', 'praying', 'answered', 'closed');

ALTER TABLE prayer_requests
    DROP CONSTRAINT IF EXISTS chk_prayer_requests_status;

ALTER TABLE prayer_requests
    ADD CONSTRAINT chk_prayer_requests_status
    CHECK (status IN ('pending', 'praying', 'answered', 'closed'));

CREATE INDEX IF NOT EXISTS idx_prayer_requests_category_created
    ON prayer_requests(category, created_at DESC)
    WHERE deleted_at IS NULL;

-- =============================================================================
-- Formerly 007_analytics_pipeline
-- =============================================================================
ALTER TABLE events ADD COLUMN IF NOT EXISTS event_date date;

DO $$
DECLARE event_row record;
BEGIN
    FOR event_row IN
        SELECT id, date FROM events
        WHERE event_date IS NULL AND date ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$'
    LOOP
        BEGIN
            UPDATE events SET event_date = event_row.date::date WHERE id = event_row.id;
        EXCEPTION WHEN datetime_field_overflow OR invalid_datetime_format THEN
            RAISE WARNING 'Skipping invalid legacy event date for event %: %', event_row.id, event_row.date;
        END;
    END LOOP;
END $$;

CREATE INDEX IF NOT EXISTS idx_events_event_date ON events(event_date);
CREATE INDEX IF NOT EXISTS idx_events_category_event_date ON events(category, event_date);

CREATE OR REPLACE FUNCTION sync_event_native_date() RETURNS trigger AS $$
BEGIN
    NEW.event_date := NEW.date::date;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_events_sync_native_date ON events;
CREATE TRIGGER trg_events_sync_native_date
BEFORE INSERT OR UPDATE OF date ON events
FOR EACH ROW EXECUTE FUNCTION sync_event_native_date();

-- Make retries idempotent before normalized events reference the batch key.
DELETE FROM analytics_batches older
USING analytics_batches newer
WHERE older.batch_id = newer.batch_id
  AND (older.created_at, older.id) > (newer.created_at, newer.id);

DROP INDEX IF EXISTS idx_analytics_batches_batch_id;
CREATE UNIQUE INDEX IF NOT EXISTS idx_analytics_batches_batch_id_unique
    ON analytics_batches(batch_id);

ALTER TABLE analytics_batches
    ADD COLUMN IF NOT EXISTS expires_at timestamptz;
UPDATE analytics_batches SET expires_at = created_at + INTERVAL '30 days' WHERE expires_at IS NULL;
ALTER TABLE analytics_batches ALTER COLUMN expires_at SET DEFAULT (NOW() + INTERVAL '30 days');
ALTER TABLE analytics_batches ALTER COLUMN expires_at SET NOT NULL;
CREATE INDEX IF NOT EXISTS idx_analytics_batches_expires_at ON analytics_batches(expires_at);

CREATE TABLE IF NOT EXISTS analytics_events (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id        varchar(120) NOT NULL,
    session_id      varchar(120) NOT NULL,
    user_id         varchar(120),
    client_event_id varchar(120),
    category        varchar(80) NOT NULL,
    action          varchar(80) NOT NULL,
    occurred_at     timestamptz NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_analytics_event_category CHECK (category ~ '^[a-z0-9][a-z0-9._:-]{0,79}$'),
    CONSTRAINT chk_analytics_event_action CHECK (action ~ '^[a-z0-9][a-z0-9._:-]{0,79}$')
);

CREATE INDEX IF NOT EXISTS idx_analytics_events_occurred_at ON analytics_events(occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_analytics_events_category_time ON analytics_events(category, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_analytics_events_action_time ON analytics_events(action, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_analytics_events_session_time ON analytics_events(session_id, occurred_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_analytics_events_batch_client_id
    ON analytics_events(batch_id, client_event_id) WHERE client_event_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_form_submissions_form_created
    ON form_submissions(form_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_attendance_sessions_date_active
    ON attendance_sessions(date DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_attendance_records_session_active
    ON attendance_records(session_id) WHERE deleted_at IS NULL;

-- =============================================================================
-- Formerly 008_new_member_workflows
-- =============================================================================
CREATE TABLE IF NOT EXISTS new_member_workflows (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    submission_id uuid NOT NULL UNIQUE REFERENCES form_submissions(id) ON DELETE CASCADE,
    stage varchar(40) NOT NULL DEFAULT 'new',
    assigned_owner_id uuid REFERENCES users(id) ON DELETE SET NULL,
    next_action_at timestamptz,
    escalation_status varchar(30) NOT NULL DEFAULT 'none',
    escalated_at timestamptz,
    completed_at timestamptz,
    last_contacted_at timestamptz,
    last_reminder_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT new_member_workflow_stage_check CHECK (stage IN ('new','contact_attempted','contacted','orientation_scheduled','orientation_completed','integrated','closed')),
    CONSTRAINT new_member_workflow_escalation_check CHECK (escalation_status IN ('none','due','escalated','resolved'))
);

CREATE INDEX IF NOT EXISTS idx_new_member_workflows_owner ON new_member_workflows(assigned_owner_id, stage);
CREATE INDEX IF NOT EXISTS idx_new_member_workflows_due ON new_member_workflows(next_action_at) WHERE completed_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_new_member_workflows_escalation ON new_member_workflows(escalation_status, next_action_at);

CREATE TABLE IF NOT EXISTS new_member_contacts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id uuid NOT NULL REFERENCES new_member_workflows(id) ON DELETE CASCADE,
    channel varchar(30) NOT NULL,
    outcome varchar(50) NOT NULL,
    notes text,
    contacted_at timestamptz NOT NULL,
    created_by_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT new_member_contact_channel_check CHECK (channel IN ('phone','email','sms','whatsapp','in_person','other'))
);
CREATE INDEX IF NOT EXISTS idx_new_member_contacts_workflow ON new_member_contacts(workflow_id, contacted_at DESC);

CREATE TABLE IF NOT EXISTS new_member_workflow_history (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id uuid NOT NULL REFERENCES new_member_workflows(id) ON DELETE CASCADE,
    event_type varchar(50) NOT NULL,
    from_stage varchar(40),
    to_stage varchar(40),
    actor_id uuid REFERENCES users(id) ON DELETE SET NULL,
    details jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_new_member_history_workflow ON new_member_workflow_history(workflow_id, created_at DESC);

INSERT INTO new_member_workflows (submission_id, next_action_at)
SELECT fs.id, fs.created_at + INTERVAL '1 day'
FROM form_submissions fs
JOIN forms f ON f.id = fs.form_id
WHERE fs.deleted_at IS NULL AND f.deleted_at IS NULL
  AND (
    LOWER(COALESCE(f.settings->>'submissionTarget', '')) = 'member'
    OR LOWER(COALESCE(f.slug, '')) = 'add-new-member'
    OR trim(regexp_replace(LOWER(COALESCE(f.title, '')), '[^a-z0-9]+', ' ', 'g')) = 'add new member'
  )
ON CONFLICT (submission_id) DO NOTHING;

-- =============================================================================
-- Formerly 009_normalize_ministry_workforce
--
-- Fail before changing data when the deployed legacy schema does not satisfy
-- this migration's contract. This produces one actionable error instead of a
-- sequence of column failures during production deployment.
-- =============================================================================
DO $$
DECLARE
    missing_columns text;
BEGIN
    SELECT string_agg(required.table_name || '.' || required.column_name, ', ' ORDER BY required.table_name, required.column_name)
    INTO missing_columns
    FROM (VALUES
        ('members', 'id'), ('members', 'email'),
        ('ministries', 'id'), ('ministries', 'name'), ('ministries', 'leader_id'),
        ('ministries', 'deleted_at'), ('ministries', 'created_at'),
        ('ministry_members', 'ministry_id'), ('ministry_members', 'member_id'),
        ('ministry_members', 'role'), ('ministry_members', 'joined_at'), ('ministry_members', 'deleted_at'),
        ('workforce_members', 'id'), ('workforce_members', 'email'),
        ('workforce_members', 'department'), ('workforce_members', 'created_at')
    ) AS required(table_name, column_name)
    LEFT JOIN information_schema.columns actual
      ON actual.table_schema = current_schema()
     AND actual.table_name = required.table_name
     AND actual.column_name = required.column_name
    WHERE actual.column_name IS NULL;

    IF missing_columns IS NOT NULL THEN
        RAISE EXCEPTION 'migration 009 schema contract failed; missing columns: %', missing_columns;
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS ministry_workforce_members (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    ministry_id uuid NOT NULL REFERENCES ministries(id) ON DELETE CASCADE,
    workforce_member_id uuid NOT NULL REFERENCES workforce_members(id) ON DELETE CASCADE,
    role varchar(30) NOT NULL DEFAULT 'member',
    title varchar(120),
    source varchar(30) NOT NULL DEFAULT 'manual',
    joined_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    CONSTRAINT ministry_workforce_role_check CHECK (role IN ('head','deputy_head','coordinator','member'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ministry_workforce_active_unique
    ON ministry_workforce_members(ministry_id, workforce_member_id)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_ministry_workforce_ministry_role
    ON ministry_workforce_members(ministry_id, role)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_ministry_workforce_member
    ON ministry_workforce_members(workforce_member_id)
    WHERE deleted_at IS NULL;

-- Materialize ministries for real workforce departments that do not already
-- have a case-insensitive ministry record. The workforce record remains the
-- authoritative person source; this only establishes organization structure.
WITH normalized_departments AS (
    SELECT
        lower(trim(w.department)) AS normalized_name,
        min(trim(w.department)) AS display_name
    FROM workforce_members w
    WHERE trim(COALESCE(w.department, '')) <> ''
    GROUP BY lower(trim(w.department))
)
INSERT INTO ministries (id, name, description, category, is_active, created_at, updated_at)
SELECT gen_random_uuid(), d.display_name, 'Created from existing workforce department assignments.', 'department', true, now(), now()
FROM normalized_departments d
WHERE NOT EXISTS (
    SELECT 1 FROM ministries m
    WHERE m.deleted_at IS NULL AND lower(trim(m.name)) = d.normalized_name
);

-- Backfill every workforce record into its matching ministry. Re-running is safe.
-- If legacy data contains duplicate active ministry names, use exactly one
-- canonical record (oldest, then UUID) rather than multiplying assignments.
WITH canonical_ministries AS (
    SELECT id, normalized_name
    FROM (
        SELECT
            m.id,
            lower(trim(m.name)) AS normalized_name,
            row_number() OVER (
                PARTITION BY lower(trim(m.name))
                ORDER BY m.created_at ASC NULLS LAST, m.id ASC
            ) AS position
        FROM ministries m
        WHERE m.deleted_at IS NULL
    ) ranked
    WHERE position = 1
)
INSERT INTO ministry_workforce_members (ministry_id, workforce_member_id, role, source, joined_at)
SELECT m.id, w.id, 'member', 'department_sync', COALESCE(w.created_at, now())
FROM workforce_members w
JOIN canonical_ministries m ON m.normalized_name = lower(trim(w.department))
WHERE trim(COALESCE(w.department, '')) <> ''
ON CONFLICT DO NOTHING;

-- Preserve the legacy ministries.leader_id only when it can be matched to an
-- actual workforce profile by an email that is unique in both source tables;
-- never infer by name or choose arbitrarily among duplicate email records.
WITH unique_member_emails AS (
    SELECT lower(trim(email)) AS normalized_email, min(id::text)::uuid AS member_id
    FROM members
    WHERE trim(COALESCE(email, '')) <> ''
    GROUP BY lower(trim(email))
    HAVING count(*) = 1
),
unique_workforce_emails AS (
    SELECT lower(trim(email)) AS normalized_email, min(id::text)::uuid AS workforce_member_id
    FROM workforce_members
    WHERE trim(COALESCE(email, '')) <> ''
    GROUP BY lower(trim(email))
    HAVING count(*) = 1
)
INSERT INTO ministry_workforce_members (ministry_id, workforce_member_id, role, source, joined_at)
SELECT m.id, workforce.workforce_member_id, 'head', 'legacy_leader', now()
FROM ministries m
JOIN unique_member_emails member_email ON member_email.member_id = m.leader_id
JOIN unique_workforce_emails workforce ON workforce.normalized_email = member_email.normalized_email
WHERE m.deleted_at IS NULL AND m.leader_id IS NOT NULL
ON CONFLICT (ministry_id, workforce_member_id) WHERE deleted_at IS NULL
DO UPDATE SET
    role = 'head',
    source = 'legacy_leader',
    updated_at = now();

-- Preserve legacy ministry membership where the member and workforce records
-- can be deterministically matched by a normalized email that is unique in
-- both tables. Collapse duplicate legacy membership rows and retain the
-- highest role before upserting, so an existing head is never downgraded.
WITH unique_member_emails AS (
    SELECT lower(trim(email)) AS normalized_email, min(id::text)::uuid AS member_id
    FROM members
    WHERE trim(COALESCE(email, '')) <> ''
    GROUP BY lower(trim(email))
    HAVING count(*) = 1
),
unique_workforce_emails AS (
    SELECT lower(trim(email)) AS normalized_email, min(id::text)::uuid AS workforce_member_id
    FROM workforce_members
    WHERE trim(COALESCE(email, '')) <> ''
    GROUP BY lower(trim(email))
    HAVING count(*) = 1
),
legacy_assignments AS (
    SELECT
        mm.ministry_id,
        workforce.workforce_member_id,
        max(CASE WHEN lower(mm.role) IN ('head', 'leader') THEN 4
                 WHEN lower(mm.role) IN ('deputy', 'assistant', 'deputy_head') THEN 3
                 WHEN lower(mm.role) = 'coordinator' THEN 2
                 ELSE 1 END) AS role_priority,
        min(mm.joined_at) AS joined_at
    FROM ministry_members mm
    JOIN unique_member_emails member_email ON member_email.member_id = mm.member_id
    JOIN unique_workforce_emails workforce ON workforce.normalized_email = member_email.normalized_email
    WHERE mm.deleted_at IS NULL
    GROUP BY mm.ministry_id, workforce.workforce_member_id
)
INSERT INTO ministry_workforce_members (ministry_id, workforce_member_id, role, source, joined_at)
SELECT legacy.ministry_id, legacy.workforce_member_id,
       CASE WHEN legacy.role_priority = 4 THEN 'head'
            WHEN legacy.role_priority = 3 THEN 'deputy_head'
            WHEN legacy.role_priority = 2 THEN 'coordinator'
            ELSE 'member' END,
       'legacy_membership', legacy.joined_at
FROM legacy_assignments legacy
ON CONFLICT (ministry_id, workforce_member_id) WHERE deleted_at IS NULL
DO UPDATE SET
    role = CASE
        WHEN ministry_workforce_members.role = 'head' OR EXCLUDED.role = 'head' THEN 'head'
        WHEN ministry_workforce_members.role = 'deputy_head' OR EXCLUDED.role = 'deputy_head' THEN 'deputy_head'
        WHEN ministry_workforce_members.role = 'coordinator' OR EXCLUDED.role = 'coordinator' THEN 'coordinator'
        ELSE 'member'
    END,
    source = CASE
        WHEN ministry_workforce_members.role = 'head' THEN ministry_workforce_members.source
        ELSE EXCLUDED.source
    END,
    joined_at = LEAST(ministry_workforce_members.joined_at, EXCLUDED.joined_at),
    updated_at = now();

-- =============================================================================
-- Formerly 010_backfill_workforce_dates
--
-- Ensure drifted production schemas have the recurring date columns even when
-- they were created before the current baseline/005 migration history.
-- =============================================================================
ALTER TABLE workforce_members
  ADD COLUMN IF NOT EXISTS birthday_month smallint,
  ADD COLUMN IF NOT EXISTS birthday_day smallint,
  ADD COLUMN IF NOT EXISTS anniversary_month smallint,
  ADD COLUMN IF NOT EXISTS anniversary_day smallint;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'workforce_birthday_month_check') THEN
    ALTER TABLE workforce_members ADD CONSTRAINT workforce_birthday_month_check CHECK (birthday_month BETWEEN 1 AND 12);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'workforce_birthday_day_check') THEN
    ALTER TABLE workforce_members ADD CONSTRAINT workforce_birthday_day_check CHECK (birthday_day BETWEEN 1 AND 31);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'workforce_anniversary_month_check') THEN
    ALTER TABLE workforce_members ADD CONSTRAINT workforce_anniversary_month_check CHECK (anniversary_month BETWEEN 1 AND 12);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'workforce_anniversary_day_check') THEN
    ALTER TABLE workforce_members ADD CONSTRAINT workforce_anniversary_day_check CHECK (anniversary_day BETWEEN 1 AND 31);
  END IF;
END $$;

-- Backfill only deterministic workforce submissions matched by normalized
-- email. Existing administrator-corrected values always win. Accepted stored
-- shapes are ISO YYYY-MM-DD and DD/MM[/YYYY] (also '-' or '.' separators).
WITH raw AS (
  SELECT fs.created_at, lower(trim(fs.email)) AS email,
    COALESCE(NULLIF(trim(fs.values->>'birthday'), ''), NULLIF(trim(fs.values->>'birthDate'), ''), NULLIF(trim(fs.values->>'birth_date'), ''), NULLIF(trim(fs.values->>'dob'), ''), NULLIF(trim(fs.values->>'dateOfBirth'), ''), NULLIF(trim(fs.values->>'date_of_birth'), '')) AS birthday,
    COALESCE(NULLIF(trim(fs.values->>'anniversary'), ''), NULLIF(trim(fs.values->>'weddingAnniversary'), ''), NULLIF(trim(fs.values->>'wedding_anniversary'), ''), NULLIF(trim(fs.values->>'anniversaryDate'), ''), NULLIF(trim(fs.values->>'anniversary_date'), '')) AS anniversary
  FROM form_submissions fs
  JOIN forms f ON f.id = fs.form_id AND f.deleted_at IS NULL
  WHERE fs.deleted_at IS NULL AND trim(COALESCE(fs.email, '')) <> ''
    AND (lower(COALESCE(f.settings->>'submissionTarget', '')) LIKE 'workforce%' OR lower(COALESCE(f.settings->>'formType', '')) = 'workforce' OR lower(COALESCE(f.slug, '')) LIKE '%workforce%')
), parsed AS (
  SELECT *,
    CASE WHEN birthday ~ '^\d{4}-\d{1,2}-\d{1,2}$' THEN split_part(birthday, '-', 2)::int WHEN birthday ~ '^\d{1,2}[/.-]\d{1,2}([/.-]\d{2,4})?$' THEN regexp_replace(birthday, '^\d{1,2}[/.-](\d{1,2}).*$', '\1')::int END AS bm,
    CASE WHEN birthday ~ '^\d{4}-\d{1,2}-\d{1,2}$' THEN split_part(birthday, '-', 3)::int WHEN birthday ~ '^\d{1,2}[/.-]\d{1,2}([/.-]\d{2,4})?$' THEN regexp_replace(birthday, '^(\d{1,2})[/.-].*$', '\1')::int END AS bd,
    CASE WHEN anniversary ~ '^\d{4}-\d{1,2}-\d{1,2}$' THEN split_part(anniversary, '-', 2)::int WHEN anniversary ~ '^\d{1,2}[/.-]\d{1,2}([/.-]\d{2,4})?$' THEN regexp_replace(anniversary, '^\d{1,2}[/.-](\d{1,2}).*$', '\1')::int END AS am,
    CASE WHEN anniversary ~ '^\d{4}-\d{1,2}-\d{1,2}$' THEN split_part(anniversary, '-', 3)::int WHEN anniversary ~ '^\d{1,2}[/.-]\d{1,2}([/.-]\d{2,4})?$' THEN regexp_replace(anniversary, '^(\d{1,2})[/.-].*$', '\1')::int END AS ad
  FROM raw
), valid AS (
  SELECT *,
    (bm BETWEEN 1 AND 12 AND bd BETWEEN 1 AND CASE bm WHEN 2 THEN 29 WHEN 4 THEN 30 WHEN 6 THEN 30 WHEN 9 THEN 30 WHEN 11 THEN 30 ELSE 31 END) AS birthday_valid,
    (am BETWEEN 1 AND 12 AND ad BETWEEN 1 AND CASE am WHEN 2 THEN 29 WHEN 4 THEN 30 WHEN 6 THEN 30 WHEN 9 THEN 30 WHEN 11 THEN 30 ELSE 31 END) AS anniversary_valid
  FROM parsed
), latest_birthday AS (
  SELECT DISTINCT ON (email) email, bm, bd FROM valid WHERE birthday_valid ORDER BY email, created_at DESC
), latest_anniversary AS (
  SELECT DISTINCT ON (email) email, am, ad FROM valid WHERE anniversary_valid ORDER BY email, created_at DESC
), dates AS (
  SELECT COALESCE(b.email, a.email) AS email, b.bm, b.bd, a.am, a.ad
  FROM latest_birthday b FULL OUTER JOIN latest_anniversary a ON a.email = b.email
)
UPDATE workforce_members w SET
  birthday_month = COALESCE(w.birthday_month, d.bm),
  birthday_day = COALESCE(w.birthday_day, d.bd),
  anniversary_month = COALESCE(w.anniversary_month, d.am),
  anniversary_day = COALESCE(w.anniversary_day, d.ad),
  updated_at = CASE WHEN (w.birthday_month IS NULL AND d.bm IS NOT NULL) OR (w.anniversary_month IS NULL AND d.am IS NOT NULL) THEN now() ELSE w.updated_at END
FROM dates d WHERE lower(trim(w.email)) = d.email;

CREATE INDEX IF NOT EXISTS idx_workforce_birthday_month_day ON workforce_members(birthday_month, birthday_day) WHERE birthday_month IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_workforce_anniversary_month_day ON workforce_members(anniversary_month, anniversary_day) WHERE anniversary_month IS NOT NULL;
-- migration: 012_visit_workflow.up.sql
-- Durable plan-a-visit workflow.
--
-- This remains a separate migration rather than being folded into schema.up.sql:
-- existing environments have already recorded the baseline migration and would
-- never execute content added to that file. Every object here is safe to create
-- on both upgraded and fresh databases.

CREATE TABLE IF NOT EXISTS visit_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    first_name VARCHAR(120) NOT NULL,
    last_name VARCHAR(120) NOT NULL,
    email VARCHAR(255) NOT NULL,
    phone VARCHAR(60),
    service_date DATE NOT NULL,
    service_at TIMESTAMPTZ NOT NULL,
    service_type VARCHAR(120) NOT NULL,
    attendance INTEGER NOT NULL DEFAULT 1 CONSTRAINT chk_visit_requests_attendance CHECK (attendance BETWEEN 1 AND 20),
    notes TEXT,
    reminder_opt_in BOOLEAN NOT NULL DEFAULT TRUE,
    status VARCHAR(40) NOT NULL DEFAULT 'new' CONSTRAINT chk_visit_requests_status CHECK (status IN ('new','confirmed','contacted','arrived','no_show','completed','cancelled')),
    assigned_to UUID REFERENCES users(id) ON DELETE SET NULL,
    next_follow_up_at TIMESTAMPTZ,
    follow_up_notified_at TIMESTAMPTZ,
    last_contact_at TIMESTAMPTZ,
    confirmation_sent_at TIMESTAMPTZ,
    confirmation_claimed_at TIMESTAMPTZ,
    reminder_sent_at TIMESTAMPTZ,
    reminder_claimed_at TIMESTAMPTZ,
    follow_up_claimed_at TIMESTAMPTZ,
    checked_in_at TIMESTAMPTZ,
    source_channel VARCHAR(120) NOT NULL DEFAULT 'frontend:web:plan-visit',
    idempotency_key VARCHAR(160) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS visit_activities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    visit_id UUID NOT NULL REFERENCES visit_requests(id) ON DELETE CASCADE,
    event_type VARCHAR(60) NOT NULL,
    from_status VARCHAR(40),
    to_status VARCHAR(40),
    actor_id UUID REFERENCES users(id) ON DELETE SET NULL,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_visit_activities_visit_created ON visit_activities(visit_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_visit_activities_actor ON visit_activities(actor_id) WHERE actor_id IS NOT NULL;

-- Case-insensitive email lookup supports idempotency/debugging without forcing
-- callers to reproduce the stored casing.
CREATE INDEX IF NOT EXISTS idx_visit_requests_email_lower ON visit_requests(LOWER(email));
CREATE INDEX IF NOT EXISTS idx_visit_requests_service_at ON visit_requests(service_at);
CREATE INDEX IF NOT EXISTS idx_visit_requests_service_date ON visit_requests(service_date);
CREATE INDEX IF NOT EXISTS idx_visit_requests_service_type ON visit_requests(service_type);
CREATE INDEX IF NOT EXISTS idx_visit_requests_status ON visit_requests(status);
CREATE INDEX IF NOT EXISTS idx_visit_requests_assigned_to ON visit_requests(assigned_to) WHERE assigned_to IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_visit_requests_reminders_due
    ON visit_requests(service_at)
    WHERE reminder_sent_at IS NULL AND status NOT IN ('cancelled', 'completed');
CREATE INDEX IF NOT EXISTS idx_visit_requests_follow_ups_due
    ON visit_requests(next_follow_up_at)
    WHERE next_follow_up_at IS NOT NULL AND follow_up_notified_at IS NULL AND status NOT IN ('completed', 'cancelled');
-- migration: 013_admin_email_scheduler.up.sql
CREATE TABLE IF NOT EXISTS admin_email_schedules (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), name varchar(160) NOT NULL,
  description varchar(500) NOT NULL DEFAULT '', status varchar(20) NOT NULL DEFAULT 'draft',
  recurrence varchar(20) NOT NULL, timezone varchar(80) NOT NULL, send_time char(5) NOT NULL,
  weekdays jsonb NOT NULL DEFAULT '[]', month_days jsonb NOT NULL DEFAULT '[]',
  start_at timestamptz NOT NULL, end_at timestamptz, next_run_at timestamptz, last_run_at timestamptz,
  compose_payload jsonb NOT NULL, subject varchar(255) NOT NULL, audience_label varchar(255) NOT NULL DEFAULT '',
  run_count integer NOT NULL DEFAULT 0, consecutive_errors integer NOT NULL DEFAULT 0, last_error text,
  claimed_at timestamptz, claimed_by varchar(120), created_by_user_id uuid,
  created_by_email varchar(255), created_by_role varchar(50), created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
  CONSTRAINT admin_email_schedules_status CHECK (status IN ('draft','active','paused','completed','failed')),
  CONSTRAINT admin_email_schedules_recurrence CHECK (recurrence IN ('once','weekly','monthly'))
);
CREATE INDEX IF NOT EXISTS idx_admin_email_schedules_due ON admin_email_schedules(next_run_at) WHERE status = 'active' AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_admin_email_schedules_status ON admin_email_schedules(status);

CREATE TABLE IF NOT EXISTS admin_email_schedule_runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), schedule_id uuid NOT NULL REFERENCES admin_email_schedules(id) ON DELETE CASCADE,
  scheduled_for timestamptz NOT NULL, status varchar(20) NOT NULL, delivery_id uuid REFERENCES admin_email_deliveries(id) ON DELETE SET NULL,
  sent integer NOT NULL DEFAULT 0, failed integer NOT NULL DEFAULT 0, error text,
  started_at timestamptz NOT NULL, completed_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(schedule_id, scheduled_for)
);
CREATE INDEX IF NOT EXISTS idx_admin_email_schedule_runs_schedule ON admin_email_schedule_runs(schedule_id, scheduled_for DESC);
-- migration: 014_admin_email_scheduler_hardening.up.sql
ALTER TABLE admin_email_schedules
  ADD COLUMN IF NOT EXISTS start_date date,
  ADD COLUMN IF NOT EXISTS end_date date,
  ADD COLUMN IF NOT EXISTS pending_occurrence_at timestamptz,
  ADD COLUMN IF NOT EXISTS version integer NOT NULL DEFAULT 1;

UPDATE admin_email_schedules
SET start_date = (start_at AT TIME ZONE timezone)::date
WHERE start_date IS NULL;

UPDATE admin_email_schedules
SET end_date = (end_at AT TIME ZONE timezone)::date
WHERE end_at IS NOT NULL AND end_date IS NULL;

ALTER TABLE admin_email_schedules ALTER COLUMN start_date SET NOT NULL;

ALTER TABLE admin_email_schedule_runs
  ADD COLUMN IF NOT EXISTS attempt integer NOT NULL DEFAULT 1;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'admin_email_schedule_runs_status') THEN
    ALTER TABLE admin_email_schedule_runs ADD CONSTRAINT admin_email_schedule_runs_status
      CHECK (status IN ('running', 'completed', 'partial', 'failed'));
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_admin_email_schedules_pending_occurrence
  ON admin_email_schedules(pending_occurrence_at)
  WHERE pending_occurrence_at IS NOT NULL AND deleted_at IS NULL;
-- migration: 015_celebration_automation.up.sql
CREATE TABLE IF NOT EXISTS celebration_automation_config (
  id varchar(40) PRIMARY KEY, enabled boolean NOT NULL DEFAULT false,
  birthday_enabled boolean NOT NULL DEFAULT true, anniversary_enabled boolean NOT NULL DEFAULT true,
  timezone varchar(80) NOT NULL, send_time char(5) NOT NULL, feb29_policy varchar(12) NOT NULL DEFAULT 'feb28',
  max_attempts integer NOT NULL DEFAULT 3, retry_minutes integer NOT NULL DEFAULT 15,
  birthday_subject varchar(180) NOT NULL, anniversary_subject varchar(180) NOT NULL,
  birthday_template_key varchar(120) NOT NULL DEFAULT 'birthday', anniversary_template_key varchar(120) NOT NULL DEFAULT 'anniversary',
  last_worker_heartbeat timestamptz, last_worker_id varchar(120),
  updated_by_user_id uuid, updated_by_email varchar(255), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT celebration_feb29_policy CHECK (feb29_policy IN ('feb28','mar1','leap_only')),
  CONSTRAINT celebration_max_attempts CHECK (max_attempts BETWEEN 1 AND 10),
  CONSTRAINT celebration_retry_minutes CHECK (retry_minutes BETWEEN 1 AND 1440)
);
INSERT INTO celebration_automation_config(id,enabled,birthday_enabled,anniversary_enabled,timezone,send_time,birthday_subject,anniversary_subject)
VALUES ('default',false,true,true,'Africa/Lagos','09:00','Happy Birthday from The Wisdom Church','Happy Wedding Anniversary from The Wisdom Church') ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS celebration_automation_runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), run_date date NOT NULL UNIQUE, timezone varchar(80) NOT NULL,
  status varchar(20) NOT NULL, attempt integer NOT NULL DEFAULT 1, targeted integer NOT NULL DEFAULT 0,
  sent integer NOT NULL DEFAULT 0, suppressed integer NOT NULL DEFAULT 0, skipped integer NOT NULL DEFAULT 0, failed integer NOT NULL DEFAULT 0,
  last_error text, next_attempt_at timestamptz, claimed_at timestamptz, claimed_by varchar(120), trigger varchar(30) NOT NULL,
  config_snapshot jsonb NOT NULL, started_at timestamptz, completed_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT celebration_run_status CHECK (status IN ('pending','running','partial','completed','failed'))
);
CREATE INDEX IF NOT EXISTS idx_celebration_runs_due ON celebration_automation_runs(run_date,next_attempt_at) WHERE status IN ('pending','partial');

CREATE TABLE IF NOT EXISTS celebration_deliveries (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), run_id uuid NOT NULL REFERENCES celebration_automation_runs(id) ON DELETE CASCADE,
  kind varchar(20) NOT NULL, email_hash char(64) NOT NULL, recipient_email varchar(255) NOT NULL, recipient_name varchar(220) NOT NULL,
  sources jsonb NOT NULL DEFAULT '[]', status varchar(20) NOT NULL, attempt integer NOT NULL DEFAULT 0, error text, sent_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(run_id,kind,email_hash),
  CONSTRAINT celebration_delivery_kind CHECK (kind IN ('birthday','anniversary')),
  CONSTRAINT celebration_delivery_status CHECK (status IN ('pending','sent','suppressed','skipped','failed'))
);
CREATE INDEX IF NOT EXISTS idx_celebration_deliveries_run_status ON celebration_deliveries(run_id,status);

-- Enforce valid calendar pairs for all future writes. NOT VALID deliberately
-- avoids blocking rollout if historical imports contain bad dates; those rows
-- remain visible for a data-quality cleanup before operators VALIDATE later.
DO $$
DECLARE table_name text; prefix text;
BEGIN
  FOREACH table_name IN ARRAY ARRAY['members','workforce_members','leadership_members'] LOOP
    prefix := replace(table_name, '_members', '');
    IF table_name = 'members' THEN prefix := 'member'; END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = prefix || '_birthday_pair_valid') THEN
      EXECUTE format('ALTER TABLE %I ADD CONSTRAINT %I CHECK ((birthday_month IS NULL AND birthday_day IS NULL) OR (birthday_month IS NOT NULL AND birthday_day IS NOT NULL AND birthday_day <= CASE birthday_month WHEN 2 THEN 29 WHEN 4 THEN 30 WHEN 6 THEN 30 WHEN 9 THEN 30 WHEN 11 THEN 30 ELSE 31 END)) NOT VALID', table_name, prefix || '_birthday_pair_valid');
    END IF;
  END LOOP;
  FOREACH table_name IN ARRAY ARRAY['workforce_members','leadership_members'] LOOP
    prefix := replace(table_name, '_members', '');
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = prefix || '_anniversary_pair_valid') THEN
      EXECUTE format('ALTER TABLE %I ADD CONSTRAINT %I CHECK ((anniversary_month IS NULL AND anniversary_day IS NULL) OR (anniversary_month IS NOT NULL AND anniversary_day IS NOT NULL AND anniversary_day <= CASE anniversary_month WHEN 2 THEN 29 WHEN 4 THEN 30 WHEN 6 THEN 30 WHEN 9 THEN 30 WHEN 11 THEN 30 ELSE 31 END)) NOT VALID', table_name, prefix || '_anniversary_pair_valid');
    END IF;
  END LOOP;
END $$;
-- migration: 016_store_checkout_lifecycle.up.sql
-- Professional store checkout lifecycle: retry safety, private customer access,
-- independent payment state, proof tracking, and idempotent stock release.
ALTER TABLE store_orders
  ADD COLUMN IF NOT EXISTS payment_status varchar(30) NOT NULL DEFAULT 'unpaid',
  ADD COLUMN IF NOT EXISTS idempotency_key varchar(100),
  ADD COLUMN IF NOT EXISTS access_token_hash varchar(64),
  ADD COLUMN IF NOT EXISTS payment_slip_url text,
  ADD COLUMN IF NOT EXISTS paid_at timestamptz,
  ADD COLUMN IF NOT EXISTS cancelled_at timestamptz,
  ADD COLUMN IF NOT EXISTS stock_released_at timestamptz;

ALTER TABLE store_orders ADD COLUMN IF NOT EXISTS reservation_expires_at timestamptz;

UPDATE store_orders SET
  idempotency_key = COALESCE(idempotency_key, id::text),
  access_token_hash = COALESCE(access_token_hash, encode(digest(id::text || order_id, 'sha256'), 'hex'));

ALTER TABLE store_orders
  ALTER COLUMN idempotency_key SET NOT NULL,
  ALTER COLUMN access_token_hash SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_store_orders_idempotency_key ON store_orders(idempotency_key);
CREATE INDEX IF NOT EXISTS idx_store_orders_payment_status ON store_orders(payment_status);
CREATE INDEX IF NOT EXISTS idx_store_orders_expiring_reservations ON store_orders(reservation_expires_at)
  WHERE status = 'pending' AND payment_status = 'unpaid' AND stock_released_at IS NULL;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_store_orders_status') THEN
    ALTER TABLE store_orders ADD CONSTRAINT chk_store_orders_status
      CHECK (status IN ('pending','processing','shipped','delivered','cancelled'));
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_store_orders_payment_status') THEN
    ALTER TABLE store_orders ADD CONSTRAINT chk_store_orders_payment_status
      CHECK (payment_status IN ('unpaid','proof_submitted','paid','failed','refunded'));
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_store_orders_payment_method') THEN
    ALTER TABLE store_orders ADD CONSTRAINT chk_store_orders_payment_method
      CHECK (payment_method IN ('transfer','delivery','online'));
  END IF;
END $$;
-- migration: 017_admin_email_recipient_results.up.sql
ALTER TABLE admin_email_deliveries
  ADD COLUMN IF NOT EXISTS recipient_results jsonb NOT NULL DEFAULT '[]'::jsonb;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_admin_email_recipient_results_array') THEN
    ALTER TABLE admin_email_deliveries
      ADD CONSTRAINT chk_admin_email_recipient_results_array
      CHECK (jsonb_typeof(recipient_results) = 'array') NOT VALID;
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_admin_email_delivery_recipient_results
  ON admin_email_deliveries USING gin (recipient_results);
-- migration: 018_form_reminder_delivery_claims.up.sql
ALTER TABLE form_calendar_reminders ADD COLUMN IF NOT EXISTS delivery_status varchar(24) NOT NULL DEFAULT 'pending';
ALTER TABLE form_calendar_reminders ADD COLUMN IF NOT EXISTS delivery_attempt int NOT NULL DEFAULT 0;
ALTER TABLE form_calendar_reminders ADD COLUMN IF NOT EXISTS claimed_at timestamptz;
ALTER TABLE form_calendar_reminders ADD COLUMN IF NOT EXISTS claimed_by varchar(120);
ALTER TABLE form_calendar_reminders ADD COLUMN IF NOT EXISTS last_error text;

UPDATE form_calendar_reminders SET delivery_status = 'provider_accepted' WHERE reminder_sent_at IS NOT NULL;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_form_reminder_delivery_status') THEN
    ALTER TABLE form_calendar_reminders ADD CONSTRAINT chk_form_reminder_delivery_status
      CHECK (delivery_status IN ('pending','processing','failed','provider_accepted')) NOT VALID;
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_form_reminders_due_delivery
  ON form_calendar_reminders(event_starts_at, delivery_status, claimed_at)
  WHERE opted_in_at IS NOT NULL AND reminder_sent_at IS NULL;
-- migration: 019_form_architecture_and_slug_aliases.up.sql
CREATE TABLE IF NOT EXISTS form_slug_aliases (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  form_id uuid NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
  slug varchar(255) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT uq_form_slug_aliases_slug UNIQUE (slug)
);

CREATE INDEX IF NOT EXISTS idx_form_slug_aliases_form_id ON form_slug_aliases(form_id);

-- Mark every existing form for the unified current renderer. The application
-- supplies the complete versioned consent defaults on read and persists them
-- the next time an administrator saves the form.
UPDATE forms
SET settings = jsonb_set(COALESCE(settings, '{}'::jsonb), '{rendererVersion}', '2'::jsonb, true),
    updated_at = now()
WHERE NOT (COALESCE(settings->>'rendererVersion', '') ~ '^[0-9]+$')
   OR (settings->>'rendererVersion')::int < 2;
-- migration: 020_celebration_automation_config_repair.up.sql
-- Repair installations where the singleton automation row was removed or an
-- earlier partial deployment created the table without its seed record.
-- The automation remains disabled until an administrator explicitly reviews
-- and activates it in the control centre.
INSERT INTO celebration_automation_config (
  id,
  enabled,
  birthday_enabled,
  anniversary_enabled,
  timezone,
  send_time,
  feb29_policy,
  max_attempts,
  retry_minutes,
  birthday_subject,
  anniversary_subject,
  birthday_template_key,
  anniversary_template_key
)
VALUES (
  'default',
  false,
  true,
  true,
  'Africa/Lagos',
  '09:00',
  'feb28',
  3,
  15,
  'Happy Birthday from The Wisdom Church',
  'Happy Wedding Anniversary from The Wisdom Church',
  'birthday',
  'anniversary'
)
ON CONFLICT (id) DO NOTHING;
