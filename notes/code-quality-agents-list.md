# Code Quality Agents

1. **Go Linting & Style Agent** — Enforces idiomatic Go style, naming conventions, formatting, and common anti-patterns (gofmt, go vet, staticcheck, revive).

2. **Error Handling Agent** — Detects unhandled or ignored errors, improper error wrapping, missing context in error chains, and panic recovery gaps.

3. **Concurrency Safety Agent** — Identifies data races, goroutine leaks, unbounded goroutine creation, improper channel usage, missing context cancellation, and mutex misuses.

4. **Nil Safety & Zero Value Agent** — Flags potential nil pointer dereferences, improper zero-value usage, and unchecked interface type assertions.

5. **Security Static Analysis Agent (SAST)** — Scans for injection flaws, XSS, command injection, path traversal, insecure cryptographic practices, hardcoded credentials, and unsafe reflection.

6. **Dependency Vulnerability Agent** — Audits go.mod dependencies against known CVE databases and suggests safe version bumps or replacements.

7. **Secrets & Sensitive Data Agent** — Detects hardcoded API keys, tokens, passwords, private keys, and any logging of sensitive personal data or credentials.

8. **Authentication & Authorization Logic Agent** — Reviews session management, JWT validation, RBAC/ABAC enforcement, missing authorization checks, and insecure cookie flags.

9. **Input Validation & Serialization Agent** — Ensures all external inputs are validated, size-limited, and properly encoded/decoded; validates JSON, XML, protobuf, and content-type handling.

10. **SQL & Database Integrity Agent** — Detects SQL injection, missing parameterization, improper transaction handling, missing context cancellation in queries, N+1 query patterns, and missing connection pool settings.

11. **Data Privacy & PII Agent** — Identifies unencrypted PII at rest or in logs, missing data anonymization, and violations of data retention or GDPR-like policies.

12. **Performance & Efficiency Analysis Agent** — Profiles CPU/memory hotspots, detects unnecessary allocations, missing slice/map preallocation, inefficient string concatenation, and misuse of sync.Pool or goroutines.

13. **Memory & Resource Leak Agent** — Detects goroutine leaks, unclosed response bodies, file handles, database connections, and potential memory retention issues from large caches or global variables.

14. **Timeout, Retry & Resilience Agent** — Checks for proper context-based timeouts, retry strategies with backoff and jitter, circuit breaker patterns, and graceful degradation in external calls.

15. **Graceful Shutdown & Health Check Agent** — Reviews server startup/shutdown ordering, OS signal handling, drain of in-flight requests, and implementation of liveness/readiness endpoints.

16. **Observability Agent** — Ensures structured logging (levels, no sensitive data), Prometheus metrics coverage, OpenTelemetry tracing, and error reporting instrumentation.

17. **API Design & Contract Agent** — Validates REST/gRPC best practices (status codes, versioning, pagination, idempotency keys), API schema consistency, and backward compatibility.

18. **Testing Adequacy Agent** — Evaluates test coverage, use of table-driven tests, race detector in CI, fakes/mocks isolation, and presence of integration tests for critical paths.

19. **Build & CI/CD Agent** — Reviews Dockerfiles for best practices (multi-stage, non-root), build cache optimization, reproducible builds, and proper use of ldflags/Go build tags.

20. **Configuration & Feature Flag Agent** — Checks that configuration is externalized (env vars/files), properly validated at startup, secrets are not in config, and feature flags have clear lifecycle policies.

21. **Dependency Health & Modularity Agent** — Analyzes go.mod hygiene (unused dependencies, replace directives, version pinning), cyclic package dependencies, and adherence to interface segregation.

22. **Documentation & Godoc Agent** — Ensures package-level comments, exported symbol docs, example functions, and contribution/deployment documentation are complete and up to date.

23. **Architecture & Domain Design Agent** — Reviews package boundaries, clean architecture layers, dependency inversion, and adherence to domain-driven design principles if applicable.

24. **Complexity & Refactoring Agent** — Measures cyclomatic complexity, function length, nesting depth, and detects code smells like duplicated logic or god objects.

25. **Licensing & Compliance Agent** — Scans for incompatible open-source licenses in dependencies, missing NOTICE files, and proprietary code disclosure risks.

26. **Internationalization (i18n) Agent** — Detects hardcoded strings, missing locale-aware formatting for dates/numbers, and improper handling of Unicode or time zones.

27. **External Service Integration Agent** — Audits third-party API clients for proper error mapping, fallback strategies, request deduplication, and secure transport (mTLS, certificate pinning).

28. **Migration & Schema Evolution Agent** — Checks database migration files for safety (no data loss, backward compatible DDL changes), rollback plans, and environment-aware migration ordering.
