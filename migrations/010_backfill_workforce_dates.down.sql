DROP INDEX IF EXISTS idx_workforce_anniversary_month_day;
DROP INDEX IF EXISTS idx_workforce_birthday_month_day;
-- Date columns are intentionally retained: dropping them would destroy user data.
