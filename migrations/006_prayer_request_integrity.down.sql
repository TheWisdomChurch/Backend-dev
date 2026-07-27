DROP INDEX IF EXISTS idx_prayer_requests_category_created;
ALTER TABLE prayer_requests DROP CONSTRAINT IF EXISTS chk_prayer_requests_status;
