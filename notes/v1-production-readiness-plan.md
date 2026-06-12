# v1.0 Production Readiness — Remaining Work

**Status:** IN PROGRESS — Coverage now **81.2%** (target was 60%)

---

## Remaining Todo

### P0 — Local Development Setup

- [ ] **Install `golangci-lint` and `staticcheck` locally** — CI runs them, but local verification is blocked without them. (~5 min)
  ```bash
  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
  go install honnef.co/go/tools/cmd/staticcheck@latest
  ```

### P1 — Risk Reduction (Production-Critical Gaps)

- [ ] **Add `module/mgr` tests (12.9% → 60%+ coverage)** — This is the highest-risk untested subsystem.
  - `NewSidecarProxy`, `TrackProcess`, `watchProcess` — completely untested
  - `Resolve`, `Spawn`, `StopAll`, `PruneCache` — all 0%
  - Extract `CommandRunner` interface from `exec.Command`, mock in tests
  - Test restart policies (Never, OnFailure, Always) and exponential backoff
- [ ] **Decompose `main()`** (908 lines) into bootstrap phases:
  `initConfig()`, `initModules()`, `initGRPC()`, `initStorage()`,
  `startServer()`. Enables testing startup sequence. (~2-3 hours)

### P2 — Code Quality (Minor/Non-Blocking)

- [x] **Extract HTTP header constants** — Done (June 9). Moved to `const` blocks in `server.go` and `middleware.go`.
- [x] **Split large interfaces** — Not needed. Actual method counts: `DatabaseProvider` (8), `StorageProvider` (7), `CacheLayer` (3). The note's 51/34/29 were incorrect. The wiki's "small capability interfaces" philosophy is already followed — capability interfaces in `storage.go:56-74` (`Streamable`, `Seekable`, `Watchable`, etc.) are already split.

### P3 — Remaining From Old Plan (Already Tracked)

- [x] Build a reference `authz` module implementing RBAC (static policy file,
      dynamic updates via event bus) — done by `auth-local` (Phases 5-6)
- [x] Chaos tests: module crash recovery loop, clock skew between nodes,
      leader election under network partition — Done (June 10). 7 new chaos tests in
      `internal/grpcmesh/chaos_test.go` and `internal/integration/chaos_test.go`.
- [x] End-to-end: 3-node cluster → join → discover → cross-node mesh call — Done (June 10).
      3 new E2E tests in `internal/integration/cluster_test.go`.
- [x] End-to-end: config SIGHUP hot-reload → subsystem picks up changes — Done (June 10).
      3 new E2E tests in `internal/integration/sighup_test.go`.
- [x] Generate OpenAPI/Swagger spec for HTTP API — `docs/openapi.yaml` (June 10)
- [x] Publish gRPC service documentation (proto comments → HTML) — `docs/grpc-services.md` (June 10)
- [x] Write v0.x → v1.0 migration guide — `docs/migration-v0-to-v1.md` (June 10)
- [x] Run final pre-release security review against previous audit findings — ✅ All 18 findings closed
      `docs/pre-release-security-review.md` (June 10). F01 remaining private-IP gap accepted:
      host allow-list + HTTPS-only provide SSRF protection; private IP blocking would break
      dev workflows (test servers run on localhost).
- [x] Tag v1.0.0-rc.1, verify all artifacts + signatures, publish

---

## Completed (June 10, 2026)

- ✅ Coverage bumped from 59.0% → 81.2% (exceeded 60% target)
- ✅ Removed committed `muxcored` binary from git
- ✅ Covered zero-coverage critical paths:
  - `audit/startSyncLoop`: 100%
  - `events/wal.go` openSegment/rotateLocked: 81.8%/66.7%
  - `grpcmesh/discovery.go` Heartbeat/StartHeartbeatLoop: 100%/100%
  - `storage/orchestrator.go` DiscoverCache/Promote/Relegate: 71.4%/75%/75%
  - `registry.go` SetState/SetHealth/StartupOrder: 100%/90%/93.5%
- ✅ **Installed `golangci-lint` v2.12.2 and `staticcheck` 2026.1 locally**
- ✅ **Extracted HTTP header constants** — `X-Forwarded-For`, `X-Real-IP`, `HX-Request`, `Origin` to `const` blocks in `server.go` and `middleware.go`
- ✅ **Added `module/mgr` tests** — 12.9% → ~70%+ (24 new tests including CommandRunner interface, Spawn, PruneCache, TrackProxy, proxy health, reconcileContracts edge cases, infoFromProto)
- ✅ **Decomposed `main()`** — 908 lines → 337 lines (54% reduction). Extracted 11 bootstrap functions: `parseFlags`, `loadConfig`, `setupContext`, `initEventBus`, `initGRPCMesh`, `initStorage`, `initHTTPServer`, `initAudit`, `initModuleManager`, `loadAndSpawnModules`, `initHealthProbes`
- ✅ **Added `MemoryBus.Close` drain tests** — coverage from 51.4% → ~80%+ (drain phase + WAL close + double-close tested)
- ✅ **Added `WAL.Close` edge case test** — coverage from 85.7% → ~90%+ (empty WAL close tested)
- ✅ **Evaluated P2 "split large interfaces"** — Not needed. Actual method counts (8/7/3) are already small and focused. Marks this task complete.
- ✅ **Documented wiki discrepancies found during review** — Appended Architecture.md bootstrap step 12 (stale admin handler) and Core-Concepts.md Fabric diagram (stale Admin UI/API Gateway boxes) to `doc-changes-for-v1.md`.
- ✅ **Chaos tests** — 7 new tests covering module crash recovery health loop, clock skew
  eviction safety (4 scenarios), leader election under partition, majority survival.
  Files: `internal/grpcmesh/chaos_test.go`, `internal/integration/chaos_test.go`.
- ✅ **E2E 3-node cluster tests** — Join → discover members across 3 real gRPC nodes,
  node leave updates member list, leader departure triggers re-election.
  File: `internal/integration/cluster_test.go`.
- ✅ **E2E SIGHUP hot-reload tests** — Config reload pipeline: safe changes applied,
  unsafe changes flagged and deferred, seed node changes detected, event published.
  File: `internal/integration/sighup_test.go`.

---

## Assessment Summary

| Area | Verdict |
|------|---------|
| Build system | ✅ Production-grade |
| Tests (no-race) | ✅ All 16 packages pass |
| Race detection | ✅ All packages race-free |
| Integration tests | ✅ Pass cleanly |
| Linting (`go vet`, `gofmt`) | ✅ Zero warnings |
| Proto definitions | ✅ Clean proto3, all documented |
| Error handling | ✅ Proper `%w` wrapping, zero panics |
| Fuzz testing | ✅ 7 fuzz functions across 4 packages |
| Security audit | ✅ Third-party audit complete |
| Vulnerability scanning | ✅ `govulncheck` in CI |
| SLSA + supply chain | ✅ SLSA provenance, cosign signing, SBOM |
| Release pipeline | ✅ GoReleaser v2, cross-compile, signed releases |
| Container config | ✅ Multi-stage, distroless, read-only root |
| Documentation | ✅ README, CHANGELOG, CONTRIBUTING, SECURITY, COMPATIBILITY |
| **Coverage** | ✅ 81.2% — *exceeds* CI threshold of 60% |
| **`module/mgr`** | ⚠️ 12.9% — highest-risk untested subsystem |
| **`main()` size** | ⚠️ 908 lines — monolithic, untestable as-is |
| **Linters locally** | ⚠️ `golangci-lint` / `staticcheck` not installed |

**Estimated effort to v1.0-rc.1: ~1 week** (interleaved, not full-time).
