# USER STORIES & BACKEND ARCHITECTURE

## Product Goal
Build a backend-first, data-driven church platform where all client applications (`frontend`, `admin`, and future channels) consume secure, versioned APIs. Business logic, workflows, and decision support must live in the backend.

## Architecture Overview
- API Layer: `Gin` HTTP API under `/api/v1`, split into public, authenticated, admin, and super-admin routes.
- Domain Layer: `internal/service` contains business rules, orchestration, approval flow, auth, form workflows, and notification logic.
- Data Access Layer: `internal/repository` encapsulates persistence with `GORM` and PostgreSQL.
- Data Layer: PostgreSQL as source of truth.
- Cache Layer: Redis for rate limiting, distributed scheduler locks, and analytics insight caching.
- Async Layer: Background jobs/schedulers plus hardened worker-pool primitives in `internal/worker`.
- Security Layer: auth middleware, role middleware, session idle timeout, device fingerprinting, CSP/security headers, CORS policy, and request throttling.

## Current Security Baseline (Implemented)
- Role-based access control for admin and super-admin APIs.
- Request ID + centralized logging middleware.
- Global and auth/OTP-specific rate limiting.
- Security headers including CSP, HSTS (prod), COOP/CORP, Permissions-Policy.
- Request body size limits to mitigate oversized payload abuse.
- Trusted proxy configuration through `SERVER_TRUSTED_PROXIES`.
- HttpOnly auth/session cookies and session inactivity enforcement.

## Core User Stories

### 1. Authentication & Identity
As a church staff user, I can register/login/logout securely with OTP and MFA support so unauthorized access is reduced.
- Backend endpoints: `/api/v1/auth/*`, `/api/v1/otp/*`
- Acceptance:
  - Login, OTP verification, token refresh, logout, and account update work consistently.
  - Rate limits apply to auth/OTP endpoints.
  - Session idle timeout invalidates stale sessions.

### 2. Admin Governance
As an admin, I can approve users, moderate content, and manage church data from backend APIs so admin UI remains thin and data-driven.
- Backend endpoints: `/api/v1/admin/users/*`, `/api/v1/admin/requests/*`, testimonial/event approvals.
- Acceptance:
  - Non-admins cannot access admin APIs.
  - Super-admin routes are protected separately.

### 3. Events & Community Content
As a member, I can view approved events/reels/testimonials and submit forms from public-facing apps.
- Backend endpoints: public event/reel/testimonial/form APIs.
- Acceptance:
  - Public endpoints are read-safe and write endpoints are validated/rate-limited.

### 4. Workforce & Leadership Operations
As church leadership, I can collect applications, approve records, and track service status from backend APIs only.
- Backend endpoints: `/api/v1/workforce/*`, `/api/v1/leadership/*`, admin management routes.
- Acceptance:
  - Application capture and approval flow persist fully in backend.
  - Birthday/anniversary automation can run without frontend involvement.

### 5. Member Engagement & Communication
As ministry staff, I can send targeted communication to members/subscribers and track delivery behavior.
- Backend endpoints: notification subscription/send, admin email compose/template APIs.
- Acceptance:
  - Audience selection and send operations are server-side.
  - Delivery records are stored for auditability.

### 6. Data-Driven Decision Support
As church leadership, I can retrieve actionable intelligence based on real platform data so decisions are made from facts.
- Backend endpoint: `/api/v1/admin/analytics/insights`
- Signal set:
  - member activation rate
  - volunteer coverage rate
  - submission trend (current vs previous 30 days)
  - upcoming event load
  - composite readiness score (0-100)
- Acceptance:
  - Endpoint returns metrics + recommendations.
  - Insight results are cached in Redis for efficient repeated access.

### 7. Enterprise Store Operations
As an admin, I can manage products, stock levels, and order lifecycle from backend APIs while the public storefront remains read-only and stock-aware.
- Backend endpoints:
  - Public: `/api/v1/store/products`, `/api/v1/store/orders`, `/api/v1/store/orders/:orderId`
  - Admin: `/api/v1/admin/store/products*`, `/api/v1/admin/store/orders*`
- Acceptance:
  - Product CRUD and activation/deactivation are admin-only.
  - Stock updates are persisted server-side.
  - Order creation performs transactional stock checks/decrements.
  - Out-of-stock products cannot be ordered.

### 8. Forms-to-Operations Pipeline
As an admin, I can create any form type (including leadership forms), collect responses, and use the data for operations and communication workflows.
- Backend endpoints: `/api/v1/admin/forms*`, `/api/v1/forms/:slug*`, `/api/v1/admin/forms/:id/submissions*`
- Acceptance:
  - Form definitions and submissions are backend-owned.
  - Submission stats and exports are available for leadership decisions.
  - Form audiences can feed admin communication workflows.

## Decision Algorithm (v1)
The backend computes a `DecisionReadinessScore` using weighted operational signals:
- 35% submission momentum
- 25% member activation
- 25% volunteer coverage
- 15% event load pressure

This gives leadership a normalized score and recommendations for intervention priorities.

## API Structure Principles
- Version every endpoint (`/api/v1/...`).
- Keep handlers thin and move logic to services.
- Keep repositories focused on persistence concerns.
- Use consistent response envelopes and status codes.
- Enforce role/security middleware at route-group level.

## Operational Standards
- Graceful shutdown for HTTP server and background loops.
- Redis locks for distributed scheduler safety.
- Background workers must be idempotent, retry-safe, and observable in logs.
- Cache is optional: system degrades safely when Redis is unavailable.

## Environment Variables to Manage Carefully
- `JWT_SECRET`
- `AUTH_SECRET_KEY`
- `DATABASE_URL` (or full DB parts config)
- `REDIS_URL`
- `SERVER_TRUSTED_PROXIES`
- `SERVER_REQUEST_BODY_MAX_BYTES`
- `RATE_LIMIT_*`

## Next Implementation Backlog (Recommended)
1. Add OpenAPI/Swagger contract checks in CI for endpoint consistency.
2. Add integration tests for auth, admin RBAC, and analytics insights endpoint.
3. Introduce async job queue persistence (Redis stream or DB-backed jobs) for guaranteed delivery workflows.
4. Add anomaly detection jobs (attendance/submission outlier alerts) for leadership dashboards.
5. Add audit log table + middleware for sensitive admin mutations.
