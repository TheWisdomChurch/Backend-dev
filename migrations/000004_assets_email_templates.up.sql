BEGIN;

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

CREATE UNIQUE INDEX IF NOT EXISTS idx_assets_object_key_unique
  ON public.assets (object_key)
  WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_assets_owner
  ON public.assets (owner_type, owner_id);

CREATE INDEX IF NOT EXISTS idx_assets_kind
  ON public.assets (kind);

CREATE INDEX IF NOT EXISTS idx_assets_status
  ON public.assets (status);

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

CREATE INDEX IF NOT EXISTS idx_email_templates_owner
  ON public.email_templates (owner_type, owner_id);

CREATE INDEX IF NOT EXISTS idx_email_templates_key
  ON public.email_templates (template_key);

CREATE UNIQUE INDEX IF NOT EXISTS idx_email_templates_key_version_unique
  ON public.email_templates (template_key, version)
  WHERE deleted_at IS NULL;

COMMIT;
