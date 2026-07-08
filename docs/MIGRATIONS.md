# Database Migrations

> **Known issue:** two migration mechanisms currently exist side by side and can collide (see
> [Known issue](#known-issue-two-competing-migration-systems) below). This doc describes the current
> state; unifying it is tracked as a follow-up.

## How it works today

The app runs its own migration pass in-process on every boot (`internal/database/migrations.go`,
invoked from `cmd/api/main.go`):

1. On startup, the app connects to the database.
2. `RunMigrations()` ensures a `schema_migrations` table exists (columns: `id`, `name`, `applied_at`).
3. It reads every `*.up.sql` file from `migrations/`, running `schema.up.sql` first if present, then the
   rest alphabetically.
4. Any file whose name isn't already recorded in `schema_migrations` gets executed and recorded — this
   makes re-running the app safe (idempotent), since already-applied files are skipped.

Separately, `docker-compose.yml` also defines a `migrate` service that runs the standard
[`golang-migrate/migrate`](https://github.com/golang-migrate/migrate) CLI against the same
`migrations/` directory, tracking its own `schema_migrations` table with a **different, incompatible
schema** (`version`, `dirty` columns instead of `id`, `name`, `applied_at`). In dev compose, this runs
before the API container boots; the API then runs its own migration pass against the same table name.

### Known issue: two competing migration systems

Because both systems write to a table literally named `schema_migrations` with incompatible column
layouts, running both against the same database is a collision risk, and it's part of why past
incidents (missing columns blocking login) happened. **Do not rely on the `docker-compose.yml` `migrate`
service and the in-process runner both being correct at once** — currently only the in-process runner
(`internal/database/migrations.go`) reflects what the app actually expects, since it's the one that runs
unconditionally on boot regardless of what else executed first.

This will be unified (single migration path, transactional execution, real down-migrations) in a
dedicated cleanup pass — see the project roadmap.

## Adding a migration

1. Create `migrations/NNN_description.up.sql`, where `NNN` is the next number after the highest existing
   one. Write idempotent SQL where practical (`ADD COLUMN IF NOT EXISTS`, etc.) since the runner has no
   transaction rollback on partial failure today.
2. Also add a matching `NNN_description.down.sql` — **note:** as of this writing only the baseline
   `schema.up.sql`/`schema.down.sql` pair has a down migration; numbered migrations after it don't yet.
   Add one anyway going forward so rollback tooling can be built on top of a consistent convention.
3. Migrations run automatically the next time the app boots — no manual step or separate deploy command.

## Running migrations manually

`make migrate` is currently **not reliable** — `cmd/api/main.go` has no CLI subcommand parsing, so
`go run cmd/api/main.go migrate` ignores the `migrate` argument and boots the full server instead of
running migrations only. To run migrations without starting the HTTP server, set `RUN_AUTOMIGRATE=true`
in the environment before running the binary — the app runs migrations, logs success, and exits.

## Verifying migrations ran

```bash
# Check which migrations have been recorded
psql "$DATABASE_URL" -c "SELECT * FROM schema_migrations ORDER BY applied_at;"
```

## Rolling back

There is no automated rollback. For the baseline schema, `migrations/schema.down.sql` exists and can be
run manually. For anything after it, write and run the reverse SQL by hand, then remove the corresponding
row from `schema_migrations`:

```sql
DELETE FROM schema_migrations WHERE name = 'NNN_description.up.sql';
```
