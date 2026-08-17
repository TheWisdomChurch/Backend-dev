-- Repair installations where the singleton automation row was removed or an
-- earlier partial deployment created the table without its seed record.
-- The automation remains disabled until an administrator explicitly reviews
-- and activates it in the control centre.
INSERT INTO celebration_automation_config (
  id,
  enabled,
  birthday_enabled,
  anniversary_enabled,
  timezone,
  send_time,
  feb29_policy,
  max_attempts,
  retry_minutes,
  birthday_subject,
  anniversary_subject,
  birthday_template_key,
  anniversary_template_key
)
VALUES (
  'default',
  false,
  true,
  true,
  'Africa/Lagos',
  '09:00',
  'feb28',
  3,
  15,
  'Happy Birthday from The Wisdom Church',
  'Happy Wedding Anniversary from The Wisdom Church',
  'birthday',
  'anniversary'
)
ON CONFLICT (id) DO NOTHING;
