# Backend Security Checklist

This is a living checklist mapped to the current codebase — implemented controls, verified against the
code they claim to describe, and what's still genuinely pending. Re-verify claims here against the code
before trusting them; this file rots the moment someone changes behavior without updating it.

## Implemented

- **CSRF protection** (`internal/middleware/csrf.go`, wired in `cmd/api/router.go`)
  - HMAC-based CSRF token signing; HttpOnly CSRF secret cookie with env-configurable name/TTL.
  - Enforced on authenticated auth routes and all admin routes; `GET /api/v1/auth/csrf-token` issues the token clients send back in the configured header (default `X-CSRF-Token`).
- **Audit logging** (`internal/middleware/audit.go`, wired via `middleware.AuditLogger("auth")` / `("admin")`)
  - Dedicated audit logs for `POST|PUT|PATCH|DELETE` on auth/admin routes, including request ID, user ID, role, IP, status, and latency.
- **JWT hardening** (`internal/middleware/auth.go`, `internal/handlers/auth.go`, `internal/config/config.go`)
  - Issuer/audience validation, future-issued token rejection, subject consistency check; tokens carry `iss`/`aud`/`sub` claims.
  - RS256 preferred (with HS256 fallback); **both `JWT_SECRET` and `AUTH_SECRET_KEY` are required to be at least 32 characters**, enforced in `internal/config/config.go:validateConfig` (previously only `AUTH_SECRET_KEY` had this check — `JWT_SECRET` was checked for non-empty only, meaning a short HS256 signing key could have shipped to production; fixed).
  - Refresh tokens use rotation with a token-family model (`internal/handlers/auth_*.go`, `internal/authutil/cookies.go`); a Redis-backed JTI blocklist supports real logout/revocation.
- **Account lockout** (`internal/service/auth_service.go`)
  - Failed logins increment `FailedLoginCount`; the account locks (`IsLocked`/`LockedUntil`) after a threshold and auto-unlocks once `LockedUntil` passes.
- **Security event logging** (`internal/service/auth_service.go` via `s.security.RecordEvent`)
  - Failed logins, TOTP/OTP challenges, geo-anomaly logins, and trusted-device changes are all recorded. **Gap:** successful login/logout/password-reset-completion are not currently recorded as events — only failures and anomalies are (see Next hardening items).
- **Rate limiting** (`internal/middleware/rate_limiter.go`, wired in `cmd/api/router.go`)
  - Dedicated stricter limiter tier (`cfg.RateLimit.Auth.*`) applied to login, register, OTP send/verify/resend, and password-reset request/confirm — separate from the global limiter.
- **Cookies** — auth/session cookies are HttpOnly, never exposed to client JS (enforced by `scripts/check_no_browser_storage.sh`).

## Next hardening items (still pending — verified gaps, not just old TODOs)

- `internal/middleware/rate_limiter.go`
  - Rate-limit keys are IP-only (`redisKey(prefix, ip)`) — no per-account key, so distributed attempts across many IPs against one account aren't throttled together.
- `internal/service/auth_service.go`
  - Security events are recorded for failures/anomalies but not for successful login/logout/password-reset completion — add those for a complete audit trail.
- `internal/repository/*` + service layer
  - Introduce authorization checks at service boundaries for sensitive entity mutations (defense in depth).
- `internal/middleware/security.go`
  - Expand CSP policy by route class and introduce a reporting endpoint if required.
- `migrations/*`
  - Add a retention policy (tables/jobs) for security events and audit logs — currently unbounded growth.
- CI/CD (`.github/workflows/`)
  - No SAST, dependency scanning, or secret scanning configured — only `docker-publish.yml` (build + `go vet` + `go test -race`) exists today.

## Production policy baseline

- Use strong secrets — enforced at config load, not just documented:
  - `JWT_SECRET` >= 32 chars
  - `AUTH_SECRET_KEY` >= 32 chars
- Set `ENVIRONMENT=production` to enforce secure cookie behavior.
- Restrict CORS origins to exact production domains only.
- Keep `DISABLE_OTP=false` and `DISABLE_LOGIN_OTP=false` in production.
- `.env.production` must never be committed (see `.gitignore` — `.env.production.example` is the tracked template).
