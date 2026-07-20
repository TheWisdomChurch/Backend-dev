# Architecture

This is a backend-first, API-driven platform. All client applications (public frontend, admin portal,
future channels) consume the same versioned REST API — business logic, workflows, and decision support
live entirely in the backend, never in a client.

## Layers

| Layer | Package | Responsibility |
|---|---|---|
| Transport | `internal/handlers` | Gin HTTP handlers — bind/validate request, call a service, shape the response. No business logic. |
| Domain | `internal/service` | Business rules, orchestration, approval flows, notification logic. |
| Persistence | `internal/repository` | GORM/PostgreSQL access. No business logic — pure data access. |
| Data | PostgreSQL | Source of truth. |
| Cache | Redis (`internal/cache`) | Rate limiting, distributed scheduler locks, analytics caching. Optional — the system degrades safely when Redis is unavailable. |
| Async | `internal/worker` | Background jobs and schedulers (birthday reminders, form cleanup, email delivery). Must be idempotent, retry-safe, and observable in logs. |
| Cross-cutting | `internal/apperror`, `internal/middleware`, `internal/validation`, `internal/sanitize`, `internal/logger` | Centralized error handling, auth/RBAC/CSRF/rate-limit middleware, request validation, HTML sanitization, structured logging. |

The API is versioned under `/api/v1`, split into public, authenticated, admin, and super-admin route groups.
Keep handlers thin — validation and response shaping only; all business logic belongs in `internal/service`.

## Request lifecycle

1. Gin router matches the route and applies the route group's middleware chain (CORS → security headers →
   request ID/logging → rate limiting → auth → CSRF → RBAC, as applicable).
2. Handler binds and validates the request body via `internal/validation` (`DisallowUnknownFields`, struct
   tag validation, structured field-level error responses).
3. Handler calls into a service, which applies business rules and calls one or more repositories.
4. Errors flow up as typed `apperror.AppError` values; the global error-handling middleware converts them
   into a consistent client-safe JSON response, keeping internal error detail out of the response body.
5. Free-text fields are sanitized (`internal/sanitize`, `bluemonday.StrictPolicy()`) before persistence.

## Authentication & security model

- JWT access tokens (RS256 with HS256 fallback), delivered as HttpOnly cookies — never exposed to
  client-side JS/localStorage (enforced by `scripts/check_no_browser_storage.sh`).
- Refresh tokens with rotation and device/family tracking; a Redis-backed JTI blocklist supports real
  logout/revocation, not just stateless expiry.
- Role-based access control for admin and super-admin routes, enforced at the route-group level.
- CSRF protection (HMAC-signed token, HttpOnly secret cookie) on authenticated mutating routes.
- Session idle timeout and device fingerprinting for anomaly detection.
- Security headers: CSP, HSTS (prod), COOP/CORP, Permissions-Policy; request body size limits; trusted
  proxy configuration via `SERVER_TRUSTED_PROXIES`.
- Global and auth/OTP-specific rate limiting.

See [SECURITY.md](SECURITY.md) for the detailed, living checklist of what's implemented vs. planned.

## Feature domains

Each domain generally has a matching handler/service/repository triplet: auth & identity, admin
governance, events & community content (testimonials, reels), workforce & leadership operations, member
engagement & notifications, giving/payments, forms (definitions, submissions, exports), store
(products/orders), cell groups, ministries, attendance, prayer requests, and analytics/decision-support.

### Decision support

`GET /api/v1/admin/analytics/insights` computes a weighted `DecisionReadinessScore` from operational
signals (35% submission momentum, 25% member activation, 25% volunteer coverage, 15% event load), cached
in Redis, to give leadership a normalized readiness score and recommendations.

## Conventions

- Version every endpoint under `/api/v1` (or the current version).
- One handler/service/repository file per domain; when a file grows large enough to mix unrelated concerns,
  split it into multiple files **within the same package**, grouped by concern (e.g. `form_service.go` →
  `form_service_public.go`, `form_service_campaign.go`, etc.; `auth.go` → `auth_oauth.go`, `auth_mfa.go`).
  This is the default — it needs no export changes and no caller updates, so it's low-risk to do incrementally.
  Only reach for a true sub-package (see `internal/service/payment/`) when the split pieces genuinely need
  their own encapsulation boundary (e.g. multiple interchangeable provider implementations behind a shared
  interface) — not just as a way to shrink a file.
- Repositories stay persistence-only; no business rules leak into them.
- Use consistent response envelopes and status codes via `pkg/utils` and `apperror`.
- Background workers must be idempotent and safe to retry.

## Environment variables to manage carefully

`JWT_SECRET`, `AUTH_SECRET_KEY`, `DATABASE_URL` (or discrete DB parts), `REDIS_URL`,
`SERVER_TRUSTED_PROXIES`, `SERVER_REQUEST_BODY_MAX_BYTES`, `RATE_LIMIT_*`. See `.env.example` for the full
list and [DEVELOPMENT.md](DEVELOPMENT.md) for local setup.
