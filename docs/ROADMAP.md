# Roadmap

A prioritized work plan for taking this backend from "solid" to "premium, world-class, well-secured
product." Every item below is grounded in something verified in the current code or docs, not generic
advice — file references are included so you can jump straight to the relevant spot.

**Where this stands today:** this is already a genuinely mature backend — JWT with rotation and a
Redis-backed revocation blocklist, CSRF, per-route rate limiting, account lockout, audit logging, RBAC,
Prometheus + OpenTelemetry, a payment abstraction over Paystack/Stripe with webhook handlers, an
approval-workflow system, and a decision-support analytics engine. The items below are additive — closing
verified gaps and adding new capability, not fixing a broken foundation.

## How to read this

Each item has:
- **Impact** — how much it moves "premium/secure/scalable" for users or ops.
- **Effort** — S (days), M (1-2 weeks), L (multi-week / needs a design decision first).

Pick items independently of tier — the tiers are a suggested order (quick wins → foundation → bigger bets),
not a hard sequence.

---

## Now — quick wins (high impact, low effort)

- [ ] **Add `golangci-lint` and `go test -race -coverprofile` to CI.**
  `make lint` and `make test-coverage` already exist locally (`makefile:54`, `makefile:49`), but
  `.github/workflows/docker-publish.yml` only runs `go vet` and `go test -race` — lint and coverage are
  invisible in CI today, so regressions there ship silently.
  *Impact: High — cheap insurance. Effort: S.*

- [ ] **Add dependency/secret scanning to CI** (`govulncheck`, `gitleaks` or GitHub's native secret
  scanning, `trivy` on the built image). Flagged explicitly as a gap in `docs/SECURITY.md` under "Next
  hardening items" — currently zero automated scanning exists beyond build+test.
  *Impact: High for a payments-handling product. Effort: S.*

- [ ] **Key rate limiting by account, not just IP** (`internal/middleware/rate_limiter.go`, `redisKey(prefix,
  ip)`). Distributed credential-stuffing across many IPs against one account currently isn't throttled
  together — documented gap in `docs/SECURITY.md`.
  *Impact: High (real attack vector for a login system). Effort: S.*

- [ ] **Complete the security-event audit trail**: successful login/logout/password-reset-completion aren't
  recorded today (`internal/service/auth_service.go`) — only failures and anomalies are. Half a trail is
  hard to investigate incidents with.
  *Impact: Med-High. Effort: S.*

## Next — foundation for "premium" (1-4 weeks)

- [ ] **Publish an OpenAPI/Swagger spec.** No `swagger`/`openapi` file exists anywhere in the repo today —
  every consumer (admin portal, future mobile app, partners) currently has to read handler code to know the
  contract. Use `swaggo/swag` annotations on existing Gin handlers (low-friction, incremental) or hand-write
  a spec for the most-used routes first.
  *Impact: High — this is what "well-structured, documented, integratable" actually looks like from the
  outside. Effort: M (annotate incrementally, doesn't block anything else).*

- [ ] **Add retention policies for `audit_logs` and `security_events`.** Both grow unbounded today —
  explicitly flagged in `docs/SECURITY.md`. Needs a decision on retention window (compliance-driven, e.g.
  12-24 months) plus a scheduled worker job (there's already a working pattern for this in
  `internal/worker/`).
  *Impact: Med (cost + compliance). Effort: S-M, needs a retention-window decision first.*

- [ ] **Push authorization checks into the service layer, not just middleware** (defense in depth) —
  flagged in `docs/SECURITY.md`. Currently RBAC is enforced at the route-group level
  (`internal/middleware/role.go`, `permissions.go`); a service-layer check means a future misrouted or
  reused handler can't accidentally skip authorization.
  *Impact: Med-High (structural, prevents a whole class of future bugs). Effort: M — touches many services,
  best done incrementally per-domain.*

- [ ] **Expand CSP policy by route class + add a violation-reporting endpoint**
  (`internal/middleware/security.go`) — currently one policy for everything.
  *Impact: Med. Effort: S-M.*

## Later — new capability (bigger bets, need a decision first)

- [ ] **WhatsApp / SMS notification channel.** Right now "WhatsApp" only exists as a social-link URL in
  email footers (`internal/config/config.go:SocialWhatsAppURL`) and as an accepted-but-unimplemented value
  in the new-member contact-channel enum (`internal/service/new_member_workflow_service.go:152` —
  `"whatsapp": true` alongside phone/email/sms, but there's no actual send path for any of them beyond
  email). This is the single biggest "premium and automated" upgrade available: giving receipts, event
  reminders, birthday/anniversary pings (the worker already does these by email —
  `internal/worker/tasks/email_task.go`), and new-member follow-up nudges could all go out over WhatsApp/SMS
  with no new domain modeling needed, just a new delivery channel behind the existing notification logic.
  **Needs a decision before starting:** which provider — WhatsApp Cloud API (Meta direct, more setup,
  cheapest at scale) vs. an aggregator like Termii/Africa's Talking (faster to ship, handles SMS+WhatsApp in
  one integration, sensible given NGN/Paystack usage elsewhere in the codebase) vs. Twilio (best docs,
  priced in USD). Also needs: template pre-approval (WhatsApp requires this), consent/opt-in tracking,
  and an inbound-webhook receiver for delivery status.
  *Impact: High — directly the "WhatsApp-style backend" you mentioned. Effort: L, blocked on a provider
  decision.*

- [ ] **API versioning discipline / deprecation policy** as the API surface grows — `/api/v1` exists
  (`docs/ARCHITECTURE.md`) but there's no documented policy yet for introducing `/v2` or deprecating
  fields. Worth writing down once the OpenAPI spec above exists, not before.
  *Impact: Med (future-proofing). Effort: S once the spec exists.*

- [ ] **Data export/delete for members** (NDPR/GDPR-style subject rights). The codebase already encrypts
  sensitive fields at rest (`members.phone_enc`, `prayer_requests.request_enc` — AES-256-GCM per
  `migrations/011_consolidated_incremental_schema.up.sql`), which is a good sign this matters to you — a
  formal export/delete endpoint is the natural next step if compliance is a real requirement, not just good
  hygiene.
  *Impact: Depends entirely on whether formal compliance is a requirement — confirm before investing here.
  Effort: M-L.*

- [ ] **Observability dashboards/alerting** on top of the Prometheus metrics and OTel traces that already
  exist (`internal/metrics/prometheus.go`, `internal/telemetry/otel.go`) — right now the instrumentation
  exists but nothing was found wiring it to Grafana/alerting in this repo (likely lives in infra, worth
  confirming it's actually connected).
  *Impact: Med — mostly an ops/incident-response improvement. Effort: M, and may already be partly done in
  `infra/`.*

---

## Suggested first pick

If you want one thing to start on: **CI hardening (lint + coverage + scanning)** is the cheapest, lowest-risk
item and pays off immediately on every PR after. If you want the one with the most visible "premium" payoff:
**WhatsApp/SMS**, but that one needs you to pick a provider before any code gets written.
