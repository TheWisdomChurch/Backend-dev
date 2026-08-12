DROP TABLE IF EXISTS celebration_deliveries;
DROP TABLE IF EXISTS celebration_automation_runs;
DROP TABLE IF EXISTS celebration_automation_config;
ALTER TABLE members DROP CONSTRAINT IF EXISTS member_birthday_pair_valid;
ALTER TABLE workforce_members DROP CONSTRAINT IF EXISTS workforce_birthday_pair_valid, DROP CONSTRAINT IF EXISTS workforce_anniversary_pair_valid;
ALTER TABLE leadership_members DROP CONSTRAINT IF EXISTS leadership_birthday_pair_valid, DROP CONSTRAINT IF EXISTS leadership_anniversary_pair_valid;
