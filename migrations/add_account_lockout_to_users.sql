-- Migration: Add account lockout tracking columns to users table
-- Purpose: Support account lockout after failed login attempts
-- Created: 2026-06-10
-- Status: CRITICAL - Blocks authentication

BEGIN;

-- Add missing columns to users table
ALTER TABLE users
ADD COLUMN IF NOT EXISTS failed_login_count INTEGER DEFAULT 0,
ADD COLUMN IF NOT EXISTS last_failed_login_at TIMESTAMP NULL,
ADD COLUMN IF NOT EXISTS is_locked BOOLEAN DEFAULT FALSE,
ADD COLUMN IF NOT EXISTS locked_until TIMESTAMP NULL;

-- Create index for faster lockout checks
CREATE INDEX IF NOT EXISTS idx_users_is_locked ON users(is_locked) WHERE is_locked = true;
CREATE INDEX IF NOT EXISTS idx_users_locked_until ON users(locked_until) WHERE locked_until IS NOT NULL;

-- Add constraint to ensure locked_until is NULL when is_locked is false
ALTER TABLE users
ADD CONSTRAINT check_locked_until_consistency
CHECK (
  (is_locked = false AND locked_until IS NULL) OR
  (is_locked = true AND locked_until IS NOT NULL) OR
  (is_locked = false AND locked_until IS NOT NULL)
);

-- Verify columns were added
SELECT column_name, data_type, is_nullable
FROM information_schema.columns
WHERE table_name = 'users'
AND column_name IN ('failed_login_count', 'last_failed_login_at', 'is_locked', 'locked_until');

COMMIT;
