-- The share-testimony form can collect a contact email/phone for follow-up,
-- but the testimonials table had nowhere to store them — silently discarded.
-- These are intentionally NOT exposed on the public testimonials API (the
-- handlers serialize this struct directly with no public/admin split), so
-- the Go model marks them json:"-".

ALTER TABLE testimonials
    ADD COLUMN IF NOT EXISTS contact_email VARCHAR(255),
    ADD COLUMN IF NOT EXISTS contact_phone VARCHAR(64);
