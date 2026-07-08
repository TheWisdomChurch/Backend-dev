# Wisdom House Backend API

REST API for Wisdom House Church, built with Go, Gin, PostgreSQL, and Redis.

## Quick Start

```bash
go mod download
cp .env.example .env      # edit with local DB/Redis credentials
go run cmd/api/main.go
```

API available at `http://localhost:8080/api/v1`.

## Stack

Go 1.25 · Gin · GORM/PostgreSQL 15 · Redis 7 · Docker

## Documentation

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — layers, request lifecycle, auth model, conventions
- [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) — local setup, workflow, debugging
- [docs/SECURITY.md](docs/SECURITY.md) — security controls checklist
- [docs/MIGRATIONS.md](docs/MIGRATIONS.md) — how database migrations work
- [infra/prod/README.md](infra/prod/README.md) — production deploy config
- [CONTRIBUTING.md](CONTRIBUTING.md) — branching, commits, PR expectations

## Common commands

```bash
make run              # run server
make watch            # run with auto-reload (air)
make test             # run tests
make test-coverage    # tests + HTML coverage report
make lint             # go fmt + go vet
make build-prod       # optimized production binary
make docker-build     # build Docker image
```

## License

See [LICENSE](LICENSE).
