# Production Readiness Review

This document records production risks found during the July 2026 backend review. It is a living
engineering backlog, not a claim that one code pass makes the service hyperscale.

## Fixed in this review

- Workforce statistics now fail explicitly when any aggregate query fails. The endpoint no longer
  returns partial, plausible-looking analytics after a database error.
- Email-template activation is atomic and serialized per logical owner/key scope. Concurrent activation
  cannot silently leave partially updated state, and version lookup errors are propagated.
- Prayer-request encryption is validated during application startup. A missing encryption secret or
  repository now prevents startup instead of creating a service that fails on its first request.

## Highest-priority remaining work

1. **Database invariants:** inventory model tags against SQL migrations, then add explicit foreign keys,
   check constraints, uniqueness rules, and indexes based on measured query plans. Startup compatibility
   DDL in `internal/database/postgre.go` should move into reviewed migrations.
2. **Request-scoped cancellation:** repository interfaces are inconsistent; several domains do not accept
   `context.Context`. Standardize context propagation from Gin through service and repository layers so
   abandoned requests release database and external-provider work.
3. **Error contract:** replace domain `errors.New`/`fmt.Errorf` responses with typed `apperror` values and
   map PostgreSQL constraint failures centrally. Clients should receive stable codes without internal text.
4. **Transactional workflows:** audit every service operation that performs multiple writes or combines a
   write with job publication. Use database transactions and an outbox pattern for durable async delivery.
5. **Observability:** add RED metrics (rate, errors, duration) per route and dependency, trace database and
   provider calls, and alert on queue age, retry exhaustion, connection-pool saturation, and migration drift.
6. **Test depth:** most transport, repository, middleware, and worker packages have no tests. Add contract,
   concurrency, migration, authorization-matrix, idempotency, and failure-injection coverage before load work.
7. **Performance evidence:** establish SLOs and representative load tests before caching or denormalizing.
   Track p50/p95/p99 latency, throughput, allocation rate, query count, lock waits, and slow query plans.
8. **Operational resilience:** exercise backup restore, zero-downtime migration, rolling deploy, graceful
   shutdown, Redis/Postgres/provider degradation, dead-letter replay, and secret rotation runbooks.

## Scale architecture direction

Keep the modular monolith until measurements show a domain needs independent scaling or isolation. Enforce
clear handler/service/repository boundaries, stateless API instances, bounded database pools, Redis only for
derivable state, idempotent jobs, and durable event publication. Premature service decomposition would add
network failure modes and consistency cost without creating capacity by itself.

Before each production release, require migrations to pass against a production-shaped snapshot, run the
full test and vet suites, validate configuration in production mode, scan dependencies and containers, and
record rollback criteria.
