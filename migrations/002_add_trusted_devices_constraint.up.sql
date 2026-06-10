-- Migration: Add unique constraint to trusted_devices table
-- Created: 2026-06-10
-- Description: Enables ON CONFLICT upsert for trusted device tracking

-- Add unique constraint on (user_id, device_id)
-- First, delete any duplicate rows to allow constraint creation
DELETE FROM trusted_devices t1
WHERE EXISTS (
  SELECT 1 FROM trusted_devices t2
  WHERE t1.user_id = t2.user_id
  AND t1.device_id = t2.device_id
  AND t1.id > t2.id
);

-- Add the unique constraint
ALTER TABLE trusted_devices
ADD CONSTRAINT uq_trusted_devices_user_device
UNIQUE (user_id, device_id);

-- Create index for faster lookups
CREATE INDEX IF NOT EXISTS idx_trusted_devices_user_id ON trusted_devices(user_id);
CREATE INDEX IF NOT EXISTS idx_trusted_devices_expires_at ON trusted_devices(expires_at) WHERE deleted_at IS NULL;

-- Log migration completion
SELECT 'Unique constraint added to trusted_devices table' as migration_status;
