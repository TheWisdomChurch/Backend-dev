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
