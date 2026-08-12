# Email Scheduler Operations

The admin email scheduler persists recurrence rules and the complete compose
payload in PostgreSQL. The API process polls due work every 30 seconds and uses
`FOR UPDATE SKIP LOCKED` plus a renewable database lease, so multiple replicas
can run safely without normally claiming the same occurrence.

## Delivery guarantees

- Each occurrence has a unique `(schedule_id, scheduled_for)` run record.
- Active workers renew ownership every minute. Worker crashes are reclaimed
  after five minutes. Delivery is
  **at-least-once** because SMTP/Brevo cannot atomically commit a provider send
  and our PostgreSQL transaction. Provider-level idempotency should be enabled
  if the configured provider adds support for it.
- Retries preserve the original occurrence identity and increment its attempt
  count. Fully failed occurrences retry after 15, 30, then 45 minutes. Partial
  deliveries are recorded but are not blindly resent, which avoids duplicating
  messages to recipients who already received the campaign. Three consecutive
  failures move the schedule to `failed`; an administrator must reactivate it.
- Audiences are resolved from forms at execution time. Unsubscribed or invalid
  recipients continue to be handled by the existing compose delivery pipeline.
- All `next_run_at` values are UTC. Recurrence calculation uses the stored IANA
  timezone and local DATE fields, including daylight-saving transitions and
  extreme UTC offsets without shifting the administrator's selected day.
- Global unsubscribes are loaded immediately before every execution and are a
  hard suppression for manual and form-derived audiences. Unsubscribe links are
  encrypted tokens and bulk messages include List-Unsubscribe headers.
- Attachment downloads reject private, loopback, link-local, multicast, and
  redirect-based internal network targets.

## Deployment

Run `make migrate` before or during deployment. Migration
`013_admin_email_scheduler.up.sql` creates schedules and occurrence history;
`014_admin_email_scheduler_hardening.up.sql` adds timezone-safe local dates,
optimistic versions, coherent retry identity, and run-status constraints. The worker starts with the API and
requires no separate cron service. PostgreSQL is required; Redis is not required
for this scheduler because its locking and leases are database-backed.

## Monitoring and recovery

Monitor application logs for `email schedule worker failed` and schedules with
`status = 'failed'`. The admin portal shows the last error and consecutive error
count. Pausing clears `next_run_at` immediately; activating recalculates the next
future occurrence from the recurrence rule.

Useful checks:

```sql
SELECT id, name, status, next_run_at, consecutive_errors, last_error
FROM admin_email_schedules
WHERE deleted_at IS NULL
ORDER BY next_run_at NULLS LAST;

SELECT schedule_id, scheduled_for, status, sent, failed, error
FROM admin_email_schedule_runs
ORDER BY scheduled_for DESC
LIMIT 100;
```
