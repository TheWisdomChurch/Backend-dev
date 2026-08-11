# Database Migrations

There is a single migration mechanism: the app runs its own migration pass in-process on boot
(`internal/database/migrations.go`, invoked from `cmd/api/main.go`). No separate migrate container or
CLI is part of the deploy path.

## How it works

1. On startup (or when explicitly running the `migrate` subcommand), the app connects to the database.
2. `RunMigrations()` ensures an `app_schema_migrations` table exists (columns: `id`, `name`, `applied_at`).
3. It reads every `*.up.sql` file from `migrations/`, running `schema.up.sql` first if present, then the
   rest alphabetically.
4. Any file whose name isn't already recorded in `app_schema_migrations` gets executed and recorded —
   each migration's SQL execution and its record insert happen inside one transaction, so a failing
   migration never leaves the schema half-applied while also being unrecorded.
5. Already-applied files are skipped, so re-running the app (or the `migrate` subcommand) is always safe.

### Table name and legacy upgrade

The tracking table is `app_schema_migrations`, not `schema_migrations` — the latter name collides with
the table the standalone [`golang-migrate/migrate`](https://github.com/golang-migrate/migrate) CLI used
to use when this repo ran a separate `migrate` service in `docker-compose.yml` (removed; see git history
if you need the old config). That service tracked its own `schema_migrations` table with an incompatible
column layout (`version`, `dirty`), which could collide with this app's own use of the same table name —
this was the root cause behind at least one past "columns don't exist" production incident.

On first boot after this change, `RunMigrations()` automatically detects a legacy `schema_migrations`
table that matches *this app's* old column shape (has an `applied_at` column) and renames it to
`app_schema_migrations` in place, preserving migration history. If a `schema_migrations` table exists
with golang-migrate's shape instead (`version`/`dirty`, no `applied_at`), it's left untouched — it's not
this app's table and isn't read from anymore.

## Running migrations

```bash
make migrate
# or directly:
go run cmd/api/main.go migrate
```

This connects to the database, runs any pending migrations, and exits — it does not start the HTTP
server or any other subsystem. (The old `RUN_AUTOMIGRATE=true` environment-variable escape hatch still
works as a fallback, but the `migrate` subcommand is the preferred way to run migrations standalone.)

In normal operation you don't need to run this manually — the app runs pending migrations automatically
every time it boots.

## File layout

The migration chain currently contains three units:

- `schema.up.sql` / `schema.down.sql` — the original baseline schema.
- `011_consolidated_incremental_schema.up.sql` / `.down.sql` — every incremental change made since the
  baseline (account lockout, refresh tokens, campuses, giving, attendance, cell groups, prayer requests,
  performance indexes, ministries, audit logs, schema-drift reconciliation, approval requests, analytics
  pipeline, new-member workflows, ministry/workforce normalization, etc.), merged into one file pair.
- `012_visit_workflow.up.sql` / `.down.sql` — the durable plan-a-visit lifecycle, including scheduling,
  ownership, reminders, check-in, and follow-up tracking.

This used to be ten separate files (`001_consolidated_incremental_schema` — itself an earlier consolidation
of 11 numbered migrations — plus `002_audit_logs` … `010_backfill_workforce_dates`). They were folded into
the single `011_consolidated_incremental_schema` pair because the long list was hard to scan for no real
benefit once all of them had shipped. **This is safe only because every statement in the consolidated file
is idempotent** (`IF NOT EXISTS`, `ON CONFLICT DO NOTHING`, or a guarded `ADD CONSTRAINT`) — so the single
file converges any database to the right end state whether it previously had none, some, or all of the ten
former files applied. If you ever need to consolidate again, keep that idempotency rule: it's what makes
squashing migrations safe instead of a source of silent drift.

Note this is different from folding new content directly into `schema.up.sql` itself, which would **not**
be safe — the runner tracks applied migrations by filename, and `schema.up.sql` is already recorded as
applied on every real environment, so any new content added under that same filename would silently never
run anywhere except a brand-new database. Consolidations always need a filename the migration table
doesn't already have a record for.

## Adding a migration

1. Create `migrations/NNN_description.up.sql`, where `NNN` is the next number after the highest existing
   one (currently `012`; the next one is `013`).
   Write idempotent SQL where practical (`ADD COLUMN IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`, etc.) —
   each file still runs as a single transaction, but idempotent SQL makes it safe to hand-run during
   debugging too, and safe to fold into a future consolidation.
2. Add a matching `migrations/NNN_description.down.sql` with the reverse operation.
3. Migrations run automatically the next time the app boots, or immediately via `make migrate`.

## Verifying migrations ran

```bash
psql "$DATABASE_URL" -c "SELECT * FROM app_schema_migrations ORDER BY applied_at;"
```

## Rolling back

There is no automated `down` runner — down migrations are written for manual use. To roll back the most
recent migration:

```bash
psql "$DATABASE_URL" -f migrations/NNN_description.down.sql
psql "$DATABASE_URL" -c "DELETE FROM app_schema_migrations WHERE name = 'NNN_description.up.sql';"
```

For the baseline schema, `migrations/schema.down.sql` exists and can be run the same way.
