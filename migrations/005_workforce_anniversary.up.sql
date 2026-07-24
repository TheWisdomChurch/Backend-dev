-- Adds wedding-anniversary tracking to workforce_members, mirroring the
-- birthday month/day columns already there and the anniversary columns
-- leadership_members already has. Without this, a workforce registration
-- form asking for an anniversary date had nowhere to store the answer.
ALTER TABLE workforce_members
  ADD COLUMN IF NOT EXISTS anniversary_month smallint CHECK (anniversary_month BETWEEN 1 AND 12),
  ADD COLUMN IF NOT EXISTS anniversary_day smallint CHECK (anniversary_day BETWEEN 1 AND 31);
