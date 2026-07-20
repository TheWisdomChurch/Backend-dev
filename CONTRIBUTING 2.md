# Contributing

## Workflow

```bash
git checkout -b feature/short-description
# make changes
make lint            # go fmt + go vet
make test            # go test -race ./...
git commit -am "Describe the change and why"
git push origin feature/short-description
```

Open a pull request against `main`. CI runs `go vet` and `go test -race ./...` before the Docker image
builds — a PR shouldn't merge with either failing.

## Code conventions

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the full layering rules. In short:

- Handlers (`internal/handlers`) stay thin: bind, validate, call a service, shape the response. No
  business logic.
- Business logic lives in `internal/service`; persistence-only code lives in `internal/repository`.
- New domains follow the existing `xxx_handler.go` / `xxx_service.go` / `xxx_repository.go` naming
  pattern.
- If a file starts mixing unrelated concerns, split it into multiple files in the same package first (see
  `form_service_*.go`, `auth_oauth.go`/`auth_mfa.go`) — that needs no export or caller changes. Reach for
  a real sub-package (`internal/service/payment/`) only when the pieces need their own encapsulation
  boundary, not just to shrink a file.
- Validate all external input via `internal/validation`; sanitize free-text fields via
  `internal/sanitize` before persistence.
- Raise errors as typed `apperror.AppError` values, not raw `errors.New` — the global error middleware
  depends on this for consistent client responses.
- Use the structured logger (`internal/logger`), not `fmt.Println`/`log.Printf`.

## Commit messages

Explain *why* the change was made, not just what changed — the diff already shows what changed.

## Tests

New or touched services/handlers should land with tests, not have coverage back-filled later. Use the
existing fixtures in `internal/testutil/` (DB/Redis harness) rather than inventing a new setup pattern.
