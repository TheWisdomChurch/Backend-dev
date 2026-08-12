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
