# Concurrency Safety Audit Report — MuxCore

**Date:** 2026-06-11
**Repository:** `/home/enderk/claude/core/`
**Scope:** 97 source files across 28 packages
**Static checks:** `go vet` — no warnings. Race detector tests — all pass.

## 1. Summary

| Category | Critical | High | Medium | Low |
|---|---|---|---|---|
| Data Races | 0 | 2 | 1 | 0 |
| Goroutine Leaks | 2 | 1 | 2 | 0 |
| Unbounded Goroutine Creation | 1 | 0 | 1 | 0 |
| Improper Channel Usage | 0 | 1 | 1 | 0 |
| Missing Context Cancellation | 0 | 1 | 1 | 2 |
| Mutex Misuses | 1 | 1 | 1 | 0 |
| **Total** | **4** | **6** | **7** | **2** |

## 2. Findings

---

### CRITICAL

#### C-1: Unbounded goroutine creation in `auditLifecycle` — every lifecycle event spawns a goroutine

**File:** `internal/module/manager.go:298-310`
**Category:** Unbounded Goroutine Creation / Goroutine Leak
**Severity:** Critical

```go
func (m *Manager) auditLifecycle(action, moduleID string, details map[string]string) {
    if m.audit == nil { return }
    if details == nil { details = make(map[string]string) }
    details["module_id"] = moduleID
    go func() {
        entry := contracts.AuditEntry{...}
        if err := m.audit.Log(context.Background(), entry); err != nil {
            slog.Error("audit log write failed", ...)
        }
    }()
}
```

**Explanation:** `auditLifecycle` is called from `Register`, `Unregister`, `InitAll`, `StartAll`, `StopAll`, `HealthCheck`. Under high-frequency module churn (100+ modules checked every 30s), goroutines pile up unboundedly. The audit `Log()` blocks on I/O. Contrast with `internal/storage/orchestrator.go:606` which uses an `auditSem` of 100.

**Conditions:** Any burst of module lifecycle events (distributed cluster, repeated health check failures).

**Recommendation:** Use bounded semaphore:

```go
var lifecycleAuditSem = make(chan struct{}, 100)
go func() {
    lifecycleAuditSem <- struct{}{}
    defer func() { <-lifecycleAuditSem }()
    // ...audit log...
}()
```

---

#### C-2: `StopAll` releases `m.processes` while goroutines still access it

**File:** `internal/module/mgr/manager.go:742-778`
**Category:** Data Race
**Severity:** Critical

```go
func (m *Manager) StopAll(ctx context.Context) error {
    m.mu.Lock()
    cmds := make([]*exec.Cmd, 0, len(m.processes))
    for id, cmd := range m.processes {
        cmds = append(cmds, cmd)
    }
    m.processes = make(map[string]*exec.Cmd)  // clears under lock
    m.mu.Unlock()
    // ... stop goroutines which still have old cmd refs
}
```

**Explanation:** `watchProcess` goroutines still hold references and will `delete(m.processes, bin.ID)` after `cmd.Wait()`. The old map is abandoned but `RestartModule` reads `m.processes` without holding lock between unlock and `Spawn()`.

**Recommendation:** Signal cancellation to `watchProcess` goroutines before clearing map.

---

#### C-3: `SpawnWithWatchdog` does not set `m.binaries`

**File:** `internal/module/mgr/manager.go:440-497`
**Category:** Data Race (design-level)
**Severity:** Critical

**Explanation:** `Spawn` sets `m.binaries[bin.ID] = bin`, but `SpawnWithWatchdog` does not. If `RestartModule` is called for a watchdog-spawned module, `hasBinary` is false and restart fails. Additionally, the `setExit` race between `watchProcess` and `TrackProxy` can cause inconsistent state.

---

#### C-4: `RestartModule` lock window between kill and spawn

**File:** `internal/module/mgr/manager.go:617-644`
**Category:** Mutex Misuse — Lock ordering
**Severity:** Critical

```go
m.mu.Lock()
cmd, hasProcess := m.processes[moduleID]
bin, hasBinary := m.binaries[moduleID]
m.mu.Unlock()   // <-- lock released

// kill process... up to 5 seconds

return m.Spawn(ctx, bin)  // <-- Spawn acquires m.mu
```

**Explanation:** Between unlock and `Spawn`, `watchProcess` may delete from `m.processes` concurrently. This allows duplicate process entries.

**Recommendation:** Keep lock across entire kill+spawn, or use channel to signal watcher.

---

### HIGH

#### H-1: `fanOutQuery` / `fanOutListAll` unbounded goroutines

**Files:** `internal/grpcmesh/discovery_query.go:46-64`, `:90-108`
**Category:** Unbounded Goroutine Creation
**Severity:** High

**Explanation:** One goroutine per cluster member with no concurrency limit. 100+ nodes = 100+ concurrent connections + goroutines.

**Fix:** Limit with `sem := make(chan struct{}, 10)`.

#### H-2: `watchProcess` goroutine leak on spawn failure

**File:** `internal/module/mgr/manager.go:426`
**Category:** Goroutine Leak
**Severity:** High

**Explanation:** If `cmd.Start()` fails but `watchProcess` is already launched, the error in `finishedProcesses` may never be delivered. Fragile deferred delivery pattern.

#### H-3: Heartbeat goroutine races with `Close()`

**File:** `internal/grpcmesh/discovery.go:471-489`
**Category:** Data Race
**Severity:** High

**Explanation:** `sendHeartbeats` reads `s.dialOpts` and `s.connPool` without lock. `SetConnPool` writes under lock. Race on shutdown.

**Fix:** Read both under `s.mu.RLock()`.

#### H-4: `ConnPool.Close()` panics on double-close

**File:** `internal/grpcmesh/connpool.go:107-120`
**Category:** Improper Channel Usage
**Severity:** High

**Explanation:** `close(p.stopCh)` without `sync.Once` guard. Second call panics.

**Fix:** Add `closeOnce sync.Once`.

#### H-5: `events.go:Subscribe` send goroutine leaks

**File:** `internal/grpcmesh/events.go:124-136`
**Category:** Goroutine Leak
**Severity:** High

**Explanation:** Serialization goroutine is blocked on `stream.Send` and `pe.done <- ...` without selecting on `ctx.Done()`. If Send hangs, goroutine leaks.

**Fix:** Add `select { case pe.done <- ...; case <-ctx.Done(): return }`.

---

### MEDIUM

#### M-1: `MemoryBus.Close()` — drain goroutines race with subscriber workers

**File:** `internal/events/memory.go:87-147`
**Category:** Data Race
**Severity:** Medium

**Explanation:** `s.ch` is never closed. Drain goroutine reads while subscriber worker may still be writing.

#### M-2: `persistTask` called with `context.Background()`

**File:** `internal/workerpool/pool.go:460-467`
**Category:** Missing Context Cancellation
**Severity:** Medium

**Explanation:** `reapStaleTasks` and `FailNodeTasks` use `context.Background()` for persistence while holding `p.mu`. I/O hang blocks all pool operations.

#### M-3: `WatchModules` handler doesn't check `ctx.Done()`

**File:** `internal/storage/orchestrator.go:645-689`
**Category:** Missing Context Cancellation
**Severity:** Medium

#### M-4: `TrackProcess` races on `p.exitErr`

**File:** `internal/module/mgr/proxy.go:32-38`
**Category:** Data Race (latent)
**Severity:** Medium

**Explanation:** `TrackProcess` and `watchProcess` both call `setExit` on the same proxy. Dead code path.

#### M-5: `cfgMu` access pattern inconsistent

**File:** `cmd/muxcored/main.go:103,235,927`
**Category:** Mutex Misuse
**Severity:** Medium

**Explanation:** `cfg` fields read without `cfgMu` in some paths.

#### M-6: `MemoryBus.Request` selects race on channel

**File:** `internal/events/memory.go:513-553`
**Category:** Missing Context Cancellation
**Severity:** Medium

**Explanation:** If both `ctx.Done()` and `ch` are ready simultaneously, reply is silently dropped.

#### M-7: Heartbeat context created per peer

**File:** `internal/grpcmesh/discovery.go:516-584`
**Category:** Context leak (mild)
**Severity:** Medium

---

### LOW

#### L-1: `health.go:Watch` uses `time.After` in loop

**File:** `internal/grpcmesh/health.go:32-44`
**Category:** Resource Leak
**Severity:** Low

**Explanation:** Use `time.NewTicker` instead to avoid timer leak on stream cancel.

#### L-2: gRPC audit logging is synchronous, contradicts documented pattern

**File:** `internal/grpcmesh/server.go:117-137`, `:178-197`
**Category:** Code Quality
**Severity:** Low

**Explanation:** `AGENTS.md` says "audit is fire-and-forget; audit calls use goroutines". gRPC handler calls `auditLogger.Log` synchronously.

---

### NEEDS HUMAN REVIEW

#### NHR-1: `events.go:Subscribe` — serialization goroutine may deadlock under flow control

**File:** `internal/grpcmesh/events.go:124-136`

**Rationale:** When `stream.Send` blocks, the goroutine blocks writing to `pe.done`. The bus worker blocks on `done` receive, holding `b.sem`. This chain prevents other subscribers from processing events.

---

## 3. Cross-cutting Observations

### Systemic Issues

1. **Audit fire-and-forget inconsistently applied.** Pattern documented at `AGENTS.md:126`, but:
   - `module/manager.go:auditLifecycle` → fire-and-forget (correct, but unbounded)
   - `storage/orchestrator.go:auditStorage` → fire-and-forget with semaphore (correct)
   - `grpcmesh/server.go:Call/StreamCall` → synchronous (violates pattern)

2. **Map ownership undocumented across goroutines.** `m.processes`, `m.proxies`, `m.pendingProcesses`, `m.finishedProcesses` in `module/mgr/manager.go` accessed from `watchProcess`, `RestartModule`, `StopAll`, `TrackProxy`. Implicit lifecycle contract makes code fragile.

3. **`go vet` and race detector pass cleanly** — all findings are design-level, not memory-race level.

### Positive Observations

1. **Consistent `sync.RWMutex`** for read-mostly data (registry, discovery, orchestrator).
2. **Semaphores** in `MemoryBus` (`runtime.NumCPU()*2`) and `Orchestrator` (100 concurrent audit).
3. **Fire-and-forget audit** with semaphore bounds in memory.go and orchestrator.go.
4. **Context propagation thorough** — most functions accept and forward `context.Context`.
5. **`sync.Once` used properly** in `DiscoveryServer.Close`, `Cluster.Start`.
6. **Atomic counters** for metrics — correctly used.
7. **`connPool.Get` double-checked locking** — correct pattern.

### Priority Fixes

1. **C-1**: Bounded semaphore for `auditLifecycle` goroutines (`module/manager.go:298`)
2. **C-2**: StopAll map race (`module/mgr/manager.go:742`)
3. **C-4**: Lock ordering in `RestartModule` (`module/mgr/manager.go:617`)
4. **H-4**: `sync.Once` for `ConnPool.Close` (`connpool.go:107`)
5. **H-5**: Context awareness in events serializer goroutine (`events.go:124`)
