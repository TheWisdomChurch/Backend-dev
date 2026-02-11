-- schema.up.sql
-- Consolidated, idempotent schema (industry-standard constraints, indexes, and FKs)
-- Version: v5 (leadership members)

BEGIN;

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
-- WORKFORCE / MEMBERS
-- =========================

CREATE TABLE IF NOT EXISTS public.workforce_members (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  first_name character varying(100) NOT NULL,
  last_name character varying(100) NOT NULL,
  email character varying(255),
  phone character varying(50),
  department character varying(120) NOT NULL,
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
  email varchar(255) UNIQUE NOT NULL,
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
  status character varying(20) DEFAULT 'pending'::character varying NOT NULL,
  bio text,
  image_url text,
  created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  CONSTRAINT leadership_members_pkey PRIMARY KEY (id),
  CONSTRAINT leadership_members_role_check
    CHECK (role IN ('associate_pastor', 'deacon', 'deaconess', 'reverend')),
  CONSTRAINT leadership_members_status_check
    CHECK (status IN ('pending', 'approved'))
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
  ADD COLUMN IF NOT EXISTS published_at timestamp with time zone;

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
END $$;

-- =========================
-- INDEXES (IDEMPOTENT)
-- =========================

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_unique
  ON public.users (email)
  WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_subscribers_email_unique
  ON public.subscribers (email)
  WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_forms_slug_unique
  ON public.forms (slug)
  WHERE slug IS NOT NULL AND deleted_at IS NULL;

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

CREATE INDEX IF NOT EXISTS idx_members_birthday_month_day
  ON public.members (birthday_month, birthday_day);

CREATE INDEX IF NOT EXISTS idx_leadership_role_status
  ON public.leadership_members (role, status);

CREATE INDEX IF NOT EXISTS idx_leadership_status
  ON public.leadership_members (status);

CREATE INDEX IF NOT EXISTS idx_leadership_email
  ON public.leadership_members (email);

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

COMMIT;
