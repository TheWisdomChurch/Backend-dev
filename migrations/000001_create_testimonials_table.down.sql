-- Drop trigger first
DROP TRIGGER IF EXISTS update_testimonials_updated_at ON testimonials;

-- Drop function
DROP FUNCTION IF EXISTS update_updated_at_column();

-- Drop indexes
DROP INDEX IF EXISTS idx_testimonials_deleted_at;
DROP INDEX IF EXISTS idx_testimonials_created_at;
DROP INDEX IF EXISTS idx_testimonials_approved;

-- Drop table
DROP TABLE IF EXISTS testimonials;
