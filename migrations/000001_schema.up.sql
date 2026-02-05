-- 000001_schema.up.sql
-- Consolidated baseline schema for WisdomHouse (golang-migrate compatible)

BEGIN;

CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Timestamp update trigger function
CREATE OR REPLACE FUNCTION public.update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = CURRENT_TIMESTAMP;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- =========================
-- TABLES
-- =========================

CREATE TABLE public.users (
  id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  first_name character varying(100) NOT NULL,
  last_name character varying(100) NOT NULL,
  email character varying(255) NOT NULL,
  password character varying(255) NOT NULL,
  role character varying(50) NOT NULL DEFAULT 'admin',
  is_active boolean NOT NULL DEFAULT true,
  admin_approved boolean NOT NULL DEFAULT true,
  failed_login_count bigint NOT NULL DEFAULT 0,
  last_failed_login_at timestamp with time zone,
  last_login_at timestamp with time zone,
  created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at timestamp with time zone
);

CREATE TABLE public.events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  title character varying(200) NOT NULL,
  short_description character varying(255) NOT NULL,
  description text NOT NULL,
  date character varying(20) NOT NULL,
  "time" character varying(50) NOT NULL,
  location character varying(255) NOT NULL,
  category character varying(30) NOT NULL,
  status character varying(20) NOT NULL DEFAULT 'upcoming',
  is_featured boolean NOT NULL DEFAULT false,
  tags text[] NOT NULL DEFAULT '{}'::text[],
  register_link text,
  speaker character varying(120),
  contact_phone character varying(40),
  image text,
  banner_image text,
  attendees bigint NOT NULL DEFAULT 0,
  image_key text,
  banner_image_key text,
  created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE public.forms (
  id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  title character varying(255) NOT NULL,
  description text,
  event_id uuid,
  slug character varying(255),
  is_published boolean NOT NULL DEFAULT false,
  status character varying(20) NOT NULL DEFAULT 'draft',
  published_at timestamp with time zone,
  settings jsonb,
  created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at timestamp with time zone
);

CREATE TABLE public.form_fields (
  id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  form_id uuid NOT NULL,
  key character varying(100) NOT NULL,
  label character varying(255) NOT NULL,
  type character varying(30) NOT NULL,
  required boolean NOT NULL DEFAULT false,
  options jsonb,
  validation jsonb,
  visibility jsonb,
  "order" integer NOT NULL DEFAULT 0,
  created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at timestamp with time zone
);

CREATE TABLE public.form_submissions (
  id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  form_id uuid NOT NULL,
  "values" jsonb NOT NULL,
  name character varying(255),
  email character varying(255),
  contact_number character varying(100),
  contact_address character varying(500),
  created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at timestamp with time zone
);

CREATE TABLE public.notifications (
  id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  type character varying(20) NOT NULL,
  subject character varying(255) NOT NULL,
  title character varying(255) NOT NULL,
  message text NOT NULL,
  event_id uuid,
  created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE public.notification_deliveries (
  id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  notification_id uuid NOT NULL,
  subscriber_id uuid NOT NULL,
  status character varying(20) NOT NULL DEFAULT 'pending',
  error_message text,
  sent_at timestamp with time zone,
  created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at timestamp with time zone
);

CREATE TABLE public.otps (
  id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  email character varying(255) NOT NULL,
  purpose character varying(120),
  code_hash character varying(64) NOT NULL,
  code_salt character varying(32) NOT NULL,
  expires_at timestamp with time zone NOT NULL,
  used_at timestamp with time zone,
  created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at timestamp with time zone
);

CREATE TABLE public.reels (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  title character varying(200) NOT NULL,
  thumbnail text NOT NULL,
  video_url text NOT NULL,
  duration character varying(20) NOT NULL DEFAULT '0:00',
  event_id uuid,
  created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE public.security_events (
  id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id uuid,
  email character varying(255),
  type character varying(100) NOT NULL,
  ip character varying(45),
  user_agent character varying(512),
  metadata jsonb,
  created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at timestamp with time zone
);

CREATE TABLE public.subscribers (
  id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  email character varying(255) NOT NULL,
  name character varying(120),
  source character varying(120),
  status character varying(20) NOT NULL DEFAULT 'active',
  created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at timestamp with time zone
);

CREATE TABLE public.testimonials (
  id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  first_name character varying(100) NOT NULL,
  last_name character varying(100) NOT NULL,
  full_name character varying(200) GENERATED ALWAYS AS ((first_name || ' ' || last_name)) STORED,
  image_url character varying(500),
  testimony text NOT NULL,
  is_anonymous boolean DEFAULT false,
  is_approved boolean DEFAULT false,
  created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at timestamp with time zone
);

CREATE TABLE public.trusted_devices (
  id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id uuid NOT NULL,
  device_id character varying(255) NOT NULL,
  label character varying(255),
  last_ip character varying(45),
  user_agent character varying(512),
  trusted boolean NOT NULL DEFAULT true,
  expires_at timestamp with time zone,
  last_seen_at timestamp with time zone,
  created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at timestamp with time zone
);

CREATE TABLE public.workforce_members (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  first_name character varying(100) NOT NULL,
  last_name character varying(100) NOT NULL,
  email character varying(255),
  phone character varying(50),
  department character varying(120) NOT NULL,
  status character varying(20) NOT NULL DEFAULT 'pending',
  notes text,
  birthday_month smallint CHECK (birthday_month BETWEEN 1 AND 12),
  birthday_day smallint CHECK (birthday_day BETWEEN 1 AND 31),
  created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE public.members (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  first_name character varying(100) NOT NULL,
  last_name character varying(100) NOT NULL,
  email character varying(255) NOT NULL,
  phone character varying(50),
  is_active boolean NOT NULL DEFAULT true,
  birthday_month smallint CHECK (birthday_month BETWEEN 1 AND 12),
  birthday_day smallint CHECK (birthday_day BETWEEN 1 AND 31),
  created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- =========================
-- INDEXES
-- =========================

CREATE UNIQUE INDEX idx_users_email ON public.users USING btree (email);
CREATE INDEX idx_users_deleted_at ON public.users USING btree (deleted_at);

CREATE INDEX idx_events_status ON public.events USING btree (status);

CREATE INDEX idx_forms_deleted_at ON public.forms USING btree (deleted_at);
CREATE INDEX idx_forms_event_id ON public.forms USING btree (event_id);
CREATE UNIQUE INDEX idx_forms_slug ON public.forms USING btree (slug);

CREATE INDEX idx_form_fields_deleted_at ON public.form_fields USING btree (deleted_at);
CREATE INDEX idx_form_fields_form_id ON public.form_fields USING btree (form_id);

CREATE INDEX idx_form_submissions_deleted_at ON public.form_submissions USING btree (deleted_at);
CREATE INDEX idx_form_submissions_form_id ON public.form_submissions USING btree (form_id);

CREATE INDEX idx_notifications_event_id ON public.notifications USING btree (event_id);

CREATE INDEX idx_notification_deliveries_deleted_at ON public.notification_deliveries USING btree (deleted_at);
CREATE INDEX idx_notification_deliveries_notification_id ON public.notification_deliveries USING btree (notification_id);
CREATE INDEX idx_notification_deliveries_subscriber_id ON public.notification_deliveries USING btree (subscriber_id);

CREATE INDEX idx_otps_deleted_at ON public.otps USING btree (deleted_at);
CREATE INDEX idx_otps_email ON public.otps USING btree (email);
CREATE INDEX idx_otps_expires_at ON public.otps USING btree (expires_at);
CREATE INDEX idx_otps_purpose ON public.otps USING btree (purpose);

CREATE INDEX idx_reels_event_id ON public.reels USING btree (event_id);

CREATE INDEX idx_security_events_deleted_at ON public.security_events USING btree (deleted_at);
CREATE INDEX idx_security_events_email ON public.security_events USING btree (email);
CREATE INDEX idx_security_events_type ON public.security_events USING btree (type);
CREATE INDEX idx_security_events_user_id ON public.security_events USING btree (user_id);

CREATE UNIQUE INDEX idx_subscribers_email ON public.subscribers USING btree (email);
CREATE INDEX idx_subscribers_deleted_at ON public.subscribers USING btree (deleted_at);

CREATE INDEX idx_testimonials_approved ON public.testimonials USING btree (is_approved);
CREATE INDEX idx_testimonials_created_at ON public.testimonials USING btree (created_at DESC);
CREATE INDEX idx_testimonials_deleted_at ON public.testimonials USING btree (deleted_at);

CREATE INDEX idx_trusted_devices_deleted_at ON public.trusted_devices USING btree (deleted_at);
CREATE INDEX idx_trusted_devices_device_id ON public.trusted_devices USING btree (device_id);
CREATE INDEX idx_trusted_devices_user_id ON public.trusted_devices USING btree (user_id);

CREATE INDEX idx_workforce_members_department ON public.workforce_members USING btree (department);
CREATE INDEX idx_workforce_members_email ON public.workforce_members USING btree (email);
CREATE INDEX idx_workforce_bday_month_day ON public.workforce_members (birthday_month, birthday_day);

CREATE UNIQUE INDEX idx_members_email ON public.members USING btree (email);
CREATE INDEX idx_members_birthday_month_day ON public.members (birthday_month, birthday_day);

-- =========================
-- TRIGGERS
-- =========================

CREATE TRIGGER update_users_updated_at
BEFORE UPDATE ON public.users
FOR EACH ROW
EXECUTE FUNCTION public.update_updated_at_column();

CREATE TRIGGER update_events_updated_at
BEFORE UPDATE ON public.events
FOR EACH ROW
EXECUTE FUNCTION public.update_updated_at_column();

CREATE TRIGGER update_forms_updated_at
BEFORE UPDATE ON public.forms
FOR EACH ROW
EXECUTE FUNCTION public.update_updated_at_column();

CREATE TRIGGER update_form_fields_updated_at
BEFORE UPDATE ON public.form_fields
FOR EACH ROW
EXECUTE FUNCTION public.update_updated_at_column();

CREATE TRIGGER update_form_submissions_updated_at
BEFORE UPDATE ON public.form_submissions
FOR EACH ROW
EXECUTE FUNCTION public.update_updated_at_column();

CREATE TRIGGER update_notification_deliveries_updated_at
BEFORE UPDATE ON public.notification_deliveries
FOR EACH ROW
EXECUTE FUNCTION public.update_updated_at_column();

CREATE TRIGGER update_otps_updated_at
BEFORE UPDATE ON public.otps
FOR EACH ROW
EXECUTE FUNCTION public.update_updated_at_column();

CREATE TRIGGER update_reels_updated_at
BEFORE UPDATE ON public.reels
FOR EACH ROW
EXECUTE FUNCTION public.update_updated_at_column();

CREATE TRIGGER update_security_events_updated_at
BEFORE UPDATE ON public.security_events
FOR EACH ROW
EXECUTE FUNCTION public.update_updated_at_column();

CREATE TRIGGER update_subscribers_updated_at
BEFORE UPDATE ON public.subscribers
FOR EACH ROW
EXECUTE FUNCTION public.update_updated_at_column();

CREATE TRIGGER update_testimonials_updated_at
BEFORE UPDATE ON public.testimonials
FOR EACH ROW
EXECUTE FUNCTION public.update_updated_at_column();

CREATE TRIGGER update_trusted_devices_updated_at
BEFORE UPDATE ON public.trusted_devices
FOR EACH ROW
EXECUTE FUNCTION public.update_updated_at_column();

CREATE TRIGGER update_workforce_members_updated_at
BEFORE UPDATE ON public.workforce_members
FOR EACH ROW
EXECUTE FUNCTION public.update_updated_at_column();

CREATE TRIGGER update_members_updated_at
BEFORE UPDATE ON public.members
FOR EACH ROW
EXECUTE FUNCTION public.update_updated_at_column();

-- =========================
-- FOREIGN KEYS
-- =========================

ALTER TABLE ONLY public.form_fields
  ADD CONSTRAINT fk_form_fields_form
  FOREIGN KEY (form_id) REFERENCES public.forms(id)
  ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE ONLY public.form_submissions
  ADD CONSTRAINT fk_form_submissions_form
  FOREIGN KEY (form_id) REFERENCES public.forms(id)
  ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE ONLY public.forms
  ADD CONSTRAINT fk_forms_event
  FOREIGN KEY (event_id) REFERENCES public.events(id)
  ON UPDATE CASCADE ON DELETE SET NULL;

ALTER TABLE ONLY public.notifications
  ADD CONSTRAINT fk_notifications_event
  FOREIGN KEY (event_id) REFERENCES public.events(id)
  ON UPDATE CASCADE ON DELETE SET NULL;

ALTER TABLE ONLY public.notification_deliveries
  ADD CONSTRAINT fk_notification_deliveries_notification
  FOREIGN KEY (notification_id) REFERENCES public.notifications(id)
  ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE ONLY public.notification_deliveries
  ADD CONSTRAINT fk_notification_deliveries_subscriber
  FOREIGN KEY (subscriber_id) REFERENCES public.subscribers(id)
  ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE ONLY public.reels
  ADD CONSTRAINT fk_reels_event
  FOREIGN KEY (event_id) REFERENCES public.events(id)
  ON UPDATE CASCADE ON DELETE SET NULL;

ALTER TABLE ONLY public.security_events
  ADD CONSTRAINT fk_security_events_user
  FOREIGN KEY (user_id) REFERENCES public.users(id)
  ON UPDATE CASCADE ON DELETE SET NULL;

ALTER TABLE ONLY public.trusted_devices
  ADD CONSTRAINT fk_trusted_devices_user
  FOREIGN KEY (user_id) REFERENCES public.users(id)
  ON UPDATE CASCADE ON DELETE CASCADE;

-- =========================
-- CHECK CONSTRAINTS
-- =========================

ALTER TABLE ONLY public.events
  ADD CONSTRAINT events_status_check
  CHECK (status IN ('upcoming','happening','past'));

ALTER TABLE ONLY public.forms
  ADD CONSTRAINT forms_status_check
  CHECK (status IN ('draft','published','invalid'));

ALTER TABLE ONLY public.notification_deliveries
  ADD CONSTRAINT notification_deliveries_status_check
  CHECK (status IN ('pending','sent','failed'));

ALTER TABLE ONLY public.subscribers
  ADD CONSTRAINT subscribers_status_check
  CHECK (status IN ('active','unsubscribed'));

ALTER TABLE ONLY public.users
  ADD CONSTRAINT users_role_check
  CHECK (role IN ('admin','editor','viewer','member','staff','super_admin'));

ALTER TABLE ONLY public.workforce_members
  ADD CONSTRAINT workforce_members_status_check
  CHECK (status IN ('pending','new','serving','not_serving'));

COMMIT;
