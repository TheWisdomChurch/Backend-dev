# Database Migrations

There is a single migration mechanism: the app runs its own migration pass in-process on boot
(`internal/database/migrations.go`, invoked from `cmd/api/main.go`). No separate migrate container or
CLI is part of the deploy path.

## How it works

1. On startup (or when explicitly running the `migrate` subcommand), the app connects to the database.
2. `RunMigrations()` ensures an `app_schema_migrations` table exists (columns: `id`, `name`, `applied_at`).
3. It reads `migrations/schema.up.sql` and splits it at `-- migration: <name>.up.sql` boundaries.
4. Each logical section retains its historical migration name. Any section whose name is not recorded in
   `app_schema_migrations` is executed and recorded in one transaction.
5. Already-applied sections are skipped. This preserves compatibility with databases that applied the old
   numbered physical files while allowing the repository to maintain only two schema files.

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

The migrations directory contains exactly two physical files:

- `schema.up.sql` — the baseline and every forward migration in dependency order. Each unit starts with a
  unique `-- migration: <historical-name>.up.sql` boundary.
- `schema.down.sql` — destructive rollback operations in reverse dependency order, wrapped in one transaction.

Logical names deliberately match the previous numbered filenames. This lets a database that already recorded
`015_celebration_automation.up.sql`, for example, skip that section without replaying it. A partially upgraded
database executes only the sections it is missing, and a fresh database executes every section in order.

## Adding a migration

1. Append a uniquely named `-- migration: NNN_description.up.sql` section to `schema.up.sql`.
2. Add its rollback at the top of `schema.down.sql`, immediately after `BEGIN;`, so rollback order remains
   the reverse of forward dependencies.
3. Keep forward SQL idempotent where PostgreSQL supports it, and never rename a section after deployment.
4. Run the database tests and then execute `make migrate` against a staging database before production.

## Verifying migrations ran

```bash
psql "$DATABASE_URL" -c "SELECT * FROM app_schema_migrations ORDER BY applied_at;"
```

## Rolling back

There is no automated down runner. `schema.down.sql` is a complete destructive rollback of the consolidated
schema and must only be run against a disposable database or as part of an explicitly approved full teardown.
Operational production rollbacks should deploy a new forward migration section that restores the required
state instead of attempting to extract and execute destructive SQL manually.
