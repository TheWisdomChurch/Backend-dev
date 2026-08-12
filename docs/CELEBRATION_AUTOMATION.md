# Celebration Automation

Birthday and wedding-anniversary delivery is a first-class, database-backed
automation. It replaces the legacy environment-configured timer and its Redis
day lock. The only production execution path is `CelebrationAutomationService`.

## Guarantees

- Configuration (enabled state, IANA timezone, local send time, retry policy,
  leap-day policy, subjects, and template keys) is stored in PostgreSQL and is
  editable through the secured admin control centre.
- One durable run exists per local calendar date. PostgreSQL row locking and a
  renewable five-minute lease coordinate multiple application replicas.
- Recipients are queried from active members, serving workforce, and approved
  leadership. Workforce and leadership anniversaries are both supported.
- The same normalized email is deduplicated within each greeting type, even if
  the person exists in several source tables. Source record IDs remain in the
  delivery audit record.
- A birthday and an anniversary on the same date are intentionally distinct
  messages; their idempotency keys include the greeting type.
- Every recipient has a durable delivery row. Successful and suppressed rows
  are never resent by retries. Only failed/pending rows are retried.
- Global newsletter unsubscribes are hard suppressions checked immediately
  before delivery. Signed unsubscribe URLs and List-Unsubscribe headers are
  included in outbound messages.
- The 29 February policy is explicit: 28 February, 1 March, or leap years only.
- Manual processing uses the same daily run and recipient idempotency records as
  automatic processing, so an administrator cannot accidentally duplicate the
  day's successful deliveries.

## Operations

Migration `015_celebration_automation.up.sql` installs the automation in a
paused state. An email administrator must review timezone, subjects, templates,
and retry policy at `/dashboard/administration/automations`, then explicitly
activate automatic delivery. This prevents a deployment from unexpectedly
contacting the full audience before configuration review.

The in-process worker polls once per minute. A separate cron or Redis instance
is not required. Multiple API replicas are supported.

Monitor `celebration_automation_runs` for `partial` or `failed` status and
`celebration_deliveries` for recipient-level failures. The admin page exposes
the same aggregate history without requiring database access.
