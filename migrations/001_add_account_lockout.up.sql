-- Migration: Add account lockout tracking columns to users table
-- Created: 2026-06-10
-- Description: Supports account lockout after failed login attempts

-- Add missing columns to users table (idempotent)
ALTER TABLE users
ADD COLUMN IF NOT EXISTS failed_login_count INTEGER DEFAULT 0,
ADD COLUMN IF NOT EXISTS last_failed_login_at TIMESTAMP NULL,
ADD COLUMN IF NOT EXISTS is_locked BOOLEAN DEFAULT FALSE,
ADD COLUMN IF NOT EXISTS locked_until TIMESTAMP NULL;

-- Create indexes for faster queries
CREATE INDEX IF NOT EXISTS idx_users_is_locked ON users(is_locked) WHERE is_locked = true;
CREATE INDEX IF NOT EXISTS idx_users_locked_until ON users(locked_until) WHERE locked_until IS NOT NULL;

-- Log migration completion
SELECT 'Account lockout columns added to users table' as migration_status;
