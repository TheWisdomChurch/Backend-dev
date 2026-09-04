-- rollback: 021_wedding_anniversaries.down.sql
-- Non-destructive by default: the legacy leadership_members/workforce_members
-- anniversary_month/day columns were never dropped, so they remain the
-- fallback. Drop the new table only on an explicit rollback.
DROP TABLE IF EXISTS wedding_anniversaries;
