-- Add encrypted phone column to members table.
-- Existing plaintext phone numbers remain in the phone column as a read fallback
-- until a maintenance-window data migration encrypts and clears them.
ALTER TABLE members ADD COLUMN IF NOT EXISTS phone_enc TEXT;
