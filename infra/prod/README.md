# Production deploy config

This directory is the version-controlled replacement for whatever currently lives at
`/home/server/apps/wisdomchurch/infra/prod` on the production server. It was rebuilt from scratch here
because the server's copy was never committed to this repo, had drifted from what the app actually does,
and was the direct cause of a deploy failure:

```
wisdom_migrate | [migrate] checking schema file...
wisdom_migrate exited with code 1
```

## Why this fixes it

The failing `wisdom_migrate` container doesn't exist anywhere in this app's source — it was a standalone
container the server's old compose file ran separately from the api, checking for some "schema file" this
app never produces. That's a structural mismatch: this app runs its migrations **in-process, on api boot**
(`internal/database/migrations.go`, called unconditionally from `cmd/api/main.go` before the HTTP server
starts accepting traffic — see `docs/MIGRATIONS.md` in the repo root). There was never supposed to be a
separate migrate container in production; the server's config just never caught up to that.

This `docker-compose.yml` has one service — `api` — pulling the same image
(`ghcr.io/thewisdomchurch/wisdom-api:main`) the old broken config already used, with no separate migrate
step required for the app to come up correctly.

## Adopting this on the server

1. SSH to the server and back up whatever's currently in `infra/prod` (rename the directory, don't delete
   it, in case anything in it needs to be reconciled — e.g. if `wisdom_maintenance` turns out to be an
   intentional service, see below).
2. Copy this `infra/prod/` directory (this `docker-compose.yml`, `Makefile`, and this `README.md`) to the
   server at the same path.
3. Copy the server's real `.env.production` (with actual secrets) into this directory — it's git-ignored
   here, never committed. `.env.production.example` at the repo root documents every variable it needs.
4. Run `make deploy`. That pulls the latest image, runs pending migrations standalone first (so a bad
   migration fails loudly before any container is swapped, not silently mid-boot), then brings up `api`.

## About the `wisdom_maintenance` orphan warning

The original failure log also warned about an orphan `wisdom_maintenance` container. This config doesn't
define a maintenance service, because nothing in this repo describes what it was for — it doesn't exist in
version control anywhere. `make deploy` passes `--remove-orphans`, so it'll be cleaned up automatically.

**If `wisdom_maintenance` was doing something intentional** (a cron job, a backup task, anything) that
config was never committed either — let me know what it was supposed to do and I'll add it to this
directory properly instead of it being silent, undocumented infra.
