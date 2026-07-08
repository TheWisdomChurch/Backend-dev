# Backend Security Checklist

This checklist is mapped to the current codebase and marks what is now enforced versus what should be completed next.

## Implemented in this pass

- `main.go`
  - CSRF protection middleware is enforced on authenticated auth routes and all admin routes.
  - New authenticated endpoint: `GET /api/v1/auth/csrf-token`.
  - Audit logging middleware added for mutating `auth` and `admin` actions.
- `internal/middleware/csrf.go`
  - HMAC-based CSRF token signing.
  - HTTP-only CSRF secret cookie with env-configurable name/TTL.
  - Automatic CSRF token header emission and strict validation on mutating requests.
- `internal/middleware/audit.go`
  - Dedicated audit logs for `POST|PUT|PATCH|DELETE` including request ID, user ID, role, IP, status, and latency.
- `internal/middleware/auth.go`
  - JWT issuer/audience validation enforced.
  - Future-issued token rejection and subject consistency check.
- `internal/handlers/auth.go`
  - JWT now includes `iss`, `aud`, and `sub` claims.
  - Auth cookie clearing also clears CSRF cookie.
  - CSRF token handler exposed for frontend/admin clients.
- `internal/config/config.go`
  - Added auth CSRF config:
    - `AUTH_CSRF_COOKIE_TTL`
    - `AUTH_CSRF_COOKIE_NAME`
    - `AUTH_CSRF_HEADER_NAME`
  - Added config validation for CSRF values.
  - CORS defaults now include `X-CSRF-Token` and `X-Request-ID` in allowed/exposed headers.
- `.env.example`
  - Added CSRF env defaults and updated CORS header defaults.
- `pkg/utils/jwt.go`
  - Removed hardcoded fallback secret.
  - JWT secret now must come from `JWT_SECRET` and be at least 32 characters.

## Required client behavior

- After login, call `GET /api/v1/auth/csrf-token` with credentials.
- Send the returned token in the configured header (default `X-CSRF-Token`) for all mutating authenticated requests.

## Next hardening items (recommended)

- `internal/middleware/rate_limiter.go`
  - Add per-account + per-IP limiter keys for login and OTP routes.
  - Add dedicated stricter limits for password reset endpoints.
- `internal/service/auth_service.go`
  - Add security-event records for successful login/logout/password reset completion.
  - Add lockout/backoff policy after repeated failed logins.
- `internal/repository/*` + service layer
  - Introduce authorization checks at service boundaries for sensitive entity mutations (defense in depth).
- `internal/middleware/security.go`
  - Expand CSP policy by route class and introduce reporting endpoint if required.
- `migrations/*`
  - Add retention policy tables/jobs for security events and audit logs.
- CI/CD
  - Add SAST, dependency scanning, and secret scanning in GitHub Actions.

## Production policy baseline

- Use strong secrets:
  - `JWT_SECRET` >= 32 chars
  - `AUTH_SECRET_KEY` >= 32 chars
- Set `ENVIRONMENT=production` to enforce secure cookie behavior.
- Restrict CORS origins to exact production domains only.
- Keep `DISABLE_OTP=false` and `DISABLE_LOGIN_OTP=false` in production.
