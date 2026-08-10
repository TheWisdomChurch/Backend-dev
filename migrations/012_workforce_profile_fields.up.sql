-- Workforce self-service form collects occupation, marital status, spouse
-- name, anniversary, and an "about me" blurb, but the table had nowhere to
-- store them — they were being silently discarded on every submission.

ALTER TABLE workforce_members
    ADD COLUMN IF NOT EXISTS occupation        VARCHAR(150),
    ADD COLUMN IF NOT EXISTS marital_status    VARCHAR(10) CHECK (marital_status IN ('married', 'single')),
    ADD COLUMN IF NOT EXISTS spouse_name       VARCHAR(150),
    ADD COLUMN IF NOT EXISTS anniversary_month SMALLINT,
    ADD COLUMN IF NOT EXISTS anniversary_day   SMALLINT,
    ADD COLUMN IF NOT EXISTS about             TEXT;
