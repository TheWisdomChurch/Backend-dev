# Email Scheduler Operations

The admin email scheduler persists recurrence rules and the complete compose
payload in PostgreSQL. The API process polls due work every 30 seconds and uses
`FOR UPDATE SKIP LOCKED` plus a ten-minute database lease, so multiple replicas
can run safely without normally claiming the same occurrence.

## Delivery guarantees

- Each occurrence has a unique `(schedule_id, scheduled_for)` run record.
- Worker crashes are reclaimed after the lease expires. Delivery is
  **at-least-once** because SMTP/Brevo cannot atomically commit a provider send
  and our PostgreSQL transaction. Provider-level idempotency should be enabled
  if the configured provider adds support for it.
- Failed occurrences retry after 15, 30, then 45 minutes. Three consecutive
  failures move the schedule to `failed`; an administrator must reactivate it.
- Audiences are resolved from forms at execution time. Unsubscribed or invalid
  recipients continue to be handled by the existing compose delivery pipeline.
- All `next_run_at` values are UTC. Recurrence calculation uses the stored IANA
  timezone, including daylight-saving transitions.

## Deployment

Run `make migrate` before or during deployment. Migration
`013_admin_email_scheduler.up.sql` creates schedules, occurrence history,
constraints, and the partial due-work index. The worker starts with the API and
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
