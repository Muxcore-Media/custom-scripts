# Error Handling Remediation Plan

Source: Error Handling Audit Report (2026-06-11) — 94 files, 95 issues

---

## Priority 1: Panic Recovery — Add `defer recover()` to all goroutines

**Rationale**: 25 goroutines, zero `recover()`. A single panic crashes the process or permanently kills a subsystem.

### 1A — Critical (do first)

| Location | Lines | What to wrap |
|---|---|---|
| `internal/events/memory.go:362` — `subscriberWorker` | 362-389 | Wrap the entire `for` loop. Release semaphore (`<-b.sem`) in defer and log panic. A panicking handler kills the subscriber permanently and leaks the semaphore. |
| `internal/module/manager.go:298` — `auditLifecycle` | 298-310 | Add `defer recover()` that logs the panic. Runs on every module lifecycle event. |
| `internal/module/mgr/manager.go:483` — `SpawnWithWatchdog` | 483-494 | Add `defer m.mu.Unlock()` + `recover()` before the mutex lock. If `proxy.setExit` panics, mutex is never released. |
| `cmd/muxcored/main.go:251` — cluster event handler | 251-304 | Wrap the `select` loop body. If `FailNodeTasks` or `ResurrectOrphan` panics, task cleanup and module resurrection for departed nodes is lost forever. |

### 1B — High (do second)

| Location | Lines | Notes |
|---|---|---|
| `internal/api/middleware.go:114` — fire-and-forget goroutine | 114 | Fix: move `<-auditSem` into the outer defer after recover check |
| `internal/module/manager.go:167` — health check loop | 167-180 | Add recover, restart loop on panic |
| `internal/module/mgr/manager.go:631` — `RestartModule` Wait goroutine | 631-633 | Add `defer close(done)` + recover |
| `internal/module/mgr/manager.go:760` — `StopAll` Wait goroutine | 760-765 | Add `defer close(done)` + recover |
| `internal/module/mgr/proxy.go:33` — `TrackProcess` | 33-38 | Add `defer p.exitMu.Unlock()` + recover |
| `internal/health/core.go:80` — health probe goroutines | 80-84 | Panic causes **deadlock** on `results` channel. Send error result in recover. |
| `internal/workerpool/dispatcher.go:47` — `d.loop(ctx)` | 47 | Panic kills task dispatch permanently |
| `internal/workerpool/pool.go:102` — `p.reaperLoop(reaperCtx)` | 102 | Panic kills stale task cleanup permanently |
| `cmd/muxcore-watchdog/main.go:91` — `cmd.Wait()` goroutine | 91 | Panic causes `run()` to block forever |
| `cmd/muxcore-watchdog/main.go:96` — `monitorConnectivity` | 96 | Panic kills failover detection |
| `cmd/muxcored/main.go:416` — SIGHUP handler | 416-469 | Panic kills config reload permanently |
| `internal/storage/orchestrator.go:615` — audit goroutine | 615-633 | Add recover + semaphore release in defer |

### 1C — Medium (do third)

All grpcmesh goroutines (auth, cluster, connpool, discovery, discovery_query, events, storage_server — ~10 goroutines), eventstore subscriber goroutine (`internal/eventstore/store.go:264`), audit export goroutines (`internal/audit/audit.go:273,290`), audit sync loop (`internal/audit/audit.go:93`), events/memory drain goroutines (line 111), `cmd/muxcored/main.go` server goroutines (lines 326, 339, 378), `sdk/go/client/client.go` goroutines (lines 206, 400, 462).

Add `defer func() { if r := recover(); r != nil { slog.ErrorContext(ctx, "panic recovered", "panic", r, "stack", string(debug.Stack())) } }()` to each.

---

## Priority 2: Fix Improper Error Wrapping

| File | Line | Current | Fix |
|---|---|---|---|
| `internal/config/config.go` | 408-409 | `errors.New("server TLS cert/key invalid")` | `fmt.Errorf("server TLS cert/key: %w", err)` |
| `internal/config/config.go` | 379-381 | `fmt.Errorf(... "is not a valid TCP address")` | Append `: %w` with original err |
| `internal/config/config.go` | 384-386 | Same pattern for GRPC.Addr | Same fix |
| `internal/grpcmesh/auth.go` | 246 | `status.Error(..., "identity extraction failed")` | Embed original err via `status.Errorf` or log correlation ID |
| `internal/grpcmesh/auth.go` | 271 | `status.Error(..., "authorization check failed")` | Same pattern |

---

## Priority 3: Add Context to Bare Error Returns

### Focus: `internal/storage/local/provider.go`

8 bare `return err` statements that should wrap with operation + key:

- Line 129: `fmt.Errorf("local: marshal meta for %q: %w", p.objPath(key), err)`
- Line 131: `fmt.Errorf("local: write meta for %q: %w", p.objPath(key), err)`
- Line 149: `fmt.Errorf("local: read meta for %q: %w", p.objPath(key), err)`
- Line 153: `fmt.Errorf("local: unmarshal meta for %q: %w", p.objPath(key), err)`
- Line 169: `fmt.Errorf("local: open %q for get: %w", p.objPath(key), err)`
- Lines 185, 188: `fmt.Errorf("local: delete object %q: %w", key, err)` and `fmt.Errorf("local: delete meta for %q: %w", key, err)`
- Line 202: `fmt.Errorf("local: move %q: %w", key, err)`
- Line 289: `fmt.Errorf("local: seek %q: %w", key, err)`
- Line 312: `fmt.Errorf("local: create temp dir: %w", err)`

### Secondary: `internal/eventstore/store.go`

- Line 248: `fmt.Errorf("eventstore: read entry at %d: %w", ss, err)`
- Line 252: `fmt.Errorf("eventstore: unmarshal entry at %d: %w", ss, err)`

### Low priority: `internal/module/manager.go`, `internal/registry/registry.go`

~6 bare returns with low user impact — wrap when refactoring.

---

## Priority 4: Fix Unhandled Errors That Mask Real Failures

| File | Line | Issue | Fix |
|---|---|---|---|
| `internal/module/mgr/manager.go` | 379 | `data, _ := os.ReadFile(binPath)` — caches zero-byte binary silently | Log the error before falling through |
| `internal/module/mgr/manager.go` | 629 | `_ = cmd.Process.Signal(os.Interrupt)` | Check error, log if already-exited or permission denied |
| `internal/module/mgr/manager.go` | 632 | `_ = cmd.Wait()` | Check and log exit status |
| `internal/module/mgr/manager.go` | 638 | `_ = cmd.Process.Kill()` | Check and log error |
| `internal/idempotency/store.go` | 139 | `s.persistFile(e)` — always returns `(true, nil)` | Log more prominently, consider returning multi-error |
| `internal/idempotency/store.go` | 234 | `os.Remove(...)` — not even logged | At minimum `slog.Warn` on failure |
| `cmd/muxcored/main.go` | 483 | `srv.Drain(drainCtx)` — unchecked | Check error, surface during shutdown |
| `cmd/muxcored/main.go` | 494 | `bus.Close()` — unchecked | Check error, surface during shutdown |
| `sdk/go/mock/mock_storage.go` | 109 | `io.Copy(buf, data)` — returns truncated data silently | Capture error, return on failure |

---

## Effort Estimate

| Priority | Changes | Files | Estimated effort |
|---|---|---|---|
| P1: Panic recovery | ~25 goroutines × 5 lines = ~125 lines | ~20 files | 2-3 hours |
| P2: Error wrapping | 5 changes, ~1 line each | 2 files | 15 minutes |
| P3: Add context | ~15 changes, ~1 line each | 3 files | 30 minutes |
| P4: Unhandled errors | ~9 changes, 1-3 lines each | 5 files | 30 minutes |
| **Total** | **~54 changes** | **~25 files** | **~3.5 hours** |

Recommend tackling in order: P1A → P1B → P2 → P3 → P1C → P4. The panic recovery work in P1A prevents 4 catastrophic crash scenarios.
