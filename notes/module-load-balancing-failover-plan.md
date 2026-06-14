# Sidecar Module Load Balancing, Failover & Work Redelivery

**Status: All 23 of 24 phases implemented.** Only Option B (broadcast-based cross-cluster discovery, explicitly v2) remains — fan-out aggregation (Option A) is in place for v1. This note contains design sketches for phases that were already built in-process; they are preserved for reference but most code snippets are stale.

## 1. Current State

The cluster foundation exists. The routing, resilience, and work-tracking layers
are partially built. This section documents what's real vs. what still needs
work.

### 1.1 Load Balancing

| Component | Status | Location |
|-----------|--------|----------|
| `LBStrategy` interface | ✅ Implemented | `pkg/contracts/mesh.go:13-18` |
| `RoundRobinStrategy` | ✅ Implemented | `internal/grpcmesh/lb.go:13-29` |
| `HealthyCandidates` filter | ✅ Implemented | `internal/grpcmesh/lb.go:34-45` |
| `Client.Call()` with LB | ✅ Implemented | `internal/grpcmesh/server.go:369-450` |
| `Client.SetLBStrategy()` | ✅ Implemented | `internal/grpcmesh/server.go:338-341` |
| Wired in main.go | ✅ Done | `cmd/muxcored/main.go:604` — `NewRoundRobinStrategy()` |
| **RandomLB** | ✅ Implemented | `lb.go:33-53` |
| **LeastLoadedLB** | ✅ Implemented | `lb.go:55-99` — local in-flight call tracking |

`Client.Call()` flow:
1. Tries local in-process dispatch via `localCall()` (line 388-390)
2. Gathers remote candidates from `cluster.Members()` filtered by module ID (lines 402-414)
3. Filters to only healthy candidates via `HealthyCandidates()` (line 422)
4. Applies `LBStrategy.Pick()` (lines 428-437)
5. Fallback: tries remaining candidates in order (lines 440-447)
6. Returns `ErrRemoteRoutingUnavailable` if none succeed (line 449)

### 1.2 Cluster Health & Heartbeat

| Component | Status | Location |
|-----------|--------|----------|
| Heartbeat loop (10s ticker) | ✅ Implemented | `discovery.go:462-576` |
| ModuleHealth sync in heartbeat | ✅ Implemented | `discovery.go:310-371` |
| `NodeInfo.ModuleHealth` map | ✅ Implemented | `contracts/cluster.go:37-46` |
| Node eviction (30s timeout) | ✅ Implemented | `discovery.go:614-668` |
| Leader election (deterministic) | ✅ Implemented | `discovery.go:672-695` — lowest ID wins |
| `Cluster.Events()` channel | ✅ Implemented | `cluster.go:76-78` |
| `pollEvents()` (15s interval) | ✅ Implemented | `cluster.go:129-187` |
| **ClusterNodeDegraded emission** | ✅ Implemented | `cluster.go:193` — emitted in `pollEvents()`; handled in `main.go:265` |
| **LeaderChanged handler** | ✅ Implemented | `main.go:275` — `case contracts.ClusterLeaderChanged:` calls `ResurrectPendingOrphans()` |
| **Cluster.Health()** | ⚠️ Stub | `cluster.go:81-83` — always returns nil. Low priority; health is checked per-node via heartbeats. |
| **Cross-cluster discovery** | ✅ Implemented | `discovery_query.go` — `FindByCapability`/`FindByRole` fan out to peers via `fanOutQuery()` |

### 1.3 Failover

| Component | Status | Location |
|-----------|--------|----------|
| SDK multi-address support | ✅ Implemented | `sdk/go/client/client.go:71` — `addrs []string` |
| SDK reconnection loop | ✅ Implemented | `client.go:247-286` — address cycling, exponential backoff |
| `DialWithAddrs()` | ✅ Implemented | `client.go:169` |
| Watchdog binary | ✅ Implemented | `cmd/muxcore-watchdog/main.go:205` lines |
| `SpawnWithWatchdog()` | ✅ Implemented | `mgr/manager.go:380-437` |
| Watchdog path CLI flag | ✅ Implemented | `cmd/muxcored/main.go:83` |
| Watchdog in main.go | ✅ Done | `main.go:731-734` |
| Module resurrection (`ResurrectOrphan`) | ✅ Implemented | `mgr/manager.go:200-233` |
| Resurrection in main.go | ✅ Done | `main.go:237` — on `ClusterNodeLeft`, if leader |

**Watchdog gaps (all resolved):**
- ✅ `checkCore()` now uses **gRPC health probe** (`main.go:185-212`)
- ✅ **Exponential backoff** on failover errors (`failover` backoff, `main.go:214-245`)
- ⚠️ No re-spawn if watchdog process itself crashes — watchdog is a lightweight binary; core restart recovers it
- ⚠️ Assumes `--muxcore-mesh-addr` flag format — modules can also use `MUXCORE_MESH_ADDR` env var via the SDK

**Resurrection gaps (all resolved):**
- ✅ `LeaderChanged` handler triggers `ResurrectPendingOrphans()` at `main.go:275`
- ⚠️ No retry if `ResurrectOrphan` fails — logged and failed modules are orphaned until next event
- ⚠️ No conflict detection — double-resurrection is prevented by the 30s eviction window; rejoining nodes sync via heartbeat

### 1.4 Work Tracking

| Component | Status | Location |
|-----------|--------|----------|
| WorkerPool in-memory | ✅ Implemented | `internal/workerpool/pool.go:451` lines |
| Task lifecycle (Pending→→Failed) | ✅ Implemented | Full state machine |
| `Submit`, `Status`, `Cancel`, `List` | ✅ Implemented | All present |
| `Reassign` with MaxRetries | ✅ Implemented | `pool.go:297-338` |
| `Heartbeat` | ✅ Implemented | `pool.go:370-385` |
| `FailNodeTasks` | ✅ Implemented | `pool.go:178-204` |
| Reaper loop (stale task timeout) | ✅ Implemented | `pool.go:117-132` |
| FileStore persistence | ✅ Implemented | `internal/workerpool/store.go:109` lines |
| Wired in main.go | ✅ Done | `main.go:140-151` |
| Cluster event → `FailNodeTasks` | ✅ Done | `main.go:226` |
| 28 tests, 8 store tests | ✅ Done | `pool_test.go`, `store_test.go` |

**Implementation status:**
| Component | Status | Location |
|-----------|--------|----------|
| **Task dispatcher / Executor loop** | ✅ Implemented | `internal/workerpool/dispatcher.go` — polls pending tasks, routes to executors via registry + mesh |
| **DeadLetterProvider** | ✅ Implemented | `internal/deadletter/store.go` — in-memory with optional file persistence |
| **IdempotencyProvider** | ✅ Implemented | `internal/idempotency/store.go` — in-memory with TTL-based reaper + optional file persistence |
| **EventStore** | ✅ Implemented | `internal/eventstore/store.go` — WAL-backed, not `internal/events/store.go` |
| **Task management API** | ✅ Implemented | `internal/api/tasks.go` — list, get, cancel, reassign |
| **Graceful drain (in-flight tasks)** | ✅ Implemented | `pool.go:117-145` — `Shutdown()` persists running tasks as pending |
| **HeartbeatInterval on WorkerTask** | ✅ Implemented | `pkg/contracts/worker.go:44-48` — field exists |
| **RetryProvider** | ✅ Implemented | `internal/retry/retry.go` — exponential backoff, configurable |

### 1.5 Registry & State Machine

| Component | Status | Location |
|-----------|--------|----------|
| `Register`, `Unregister`, `SetState` | ✅ Implemented | `internal/registry/registry.go` |
| `SetHealth` with logging | ✅ Implemented | `registry.go:182-197` |
| **State transition guards** | ✅ Implemented | `registry.go:171-178` — `validTransitions` map enforced in `SetState` |
| **`GetHealth()` convenience** | ✅ Implemented | `registry.go:215-224` |

### 1.6 Module Lifecycle

| Component | Status | Location |
|-----------|--------|----------|
| `HealthCheck()` caller-invoked | ✅ Implemented | `internal/module/manager.go:143-155` |
| Degraded state on deps failure | ✅ Implemented | `manager.go:82-85, 103-106` |
| Process restart (exit-triggered) | ✅ Implemented | `mgr/manager.go:462-543` — 5 attempts, backoff |
| **Scheduled health check loop** | ✅ Implemented | `manager.go:163-181` — `StartHealthCheckLoop()` with configurable interval (default 30s) |
| **Health-triggered restart** | ✅ Implemented | `manager.go:183-201` — `healthCheckAndRemediate()` calls `RestartModule()` on the mgr manager |

---

## 2. Load Balancing

### 2.1 Current State

`Client.Call()` in `server.go:369-450` already implements LB with:
- Local-first dispatch
- `HealthyCandidates()` filtering by `ModuleHealth`
- `RoundRobinStrategy.Pick()` for selection
- Fallback loop if strategy-selected node fails

### 2.2 Missing Strategies

Add `RandomStrategy` and `LeastLoadedStrategy`:

```go
// internal/grpcmesh/lb.go

// RandomStrategy picks a candidate uniformly at random per call.
type RandomStrategy struct {
    mu sync.Mutex
    r  *rand.Rand
}

// LeastLoadedStrategy picks the node with the fewest in-flight calls.
// Requires a per-node inflight counter synced via heartbeat or tracked locally.
type LeastLoadedStrategy struct {
    inflight *atomicMap // nodeID → atomic.Int64
}
func (s *LeastLoadedStrategy) TrackCall(nodeID string)
func (s *LeastLoadedStrategy) CompleteCall(nodeID string)
```

`LeastLoadedStrategy` needs a new field on heartbeat to advertise per-node
load, or local tracking of in-flight calls per remote node (simpler for v1).

### 2.3 Cross-Cluster Discovery Aggregation

Current `DiscoveryServer.FindByCapability()` (`discovery_query.go:21`) queries
only the local registry. Two options:

**Option A — Aggregate query (v1):** The receiving node fans out to known peers
via gRPC and merges results. Simple but O(n) per query.

```go
func (s *DiscoveryServer) FindByCapability(ctx context.Context, req *discoveryv1.FindByCapabilityRequest) (*discoveryv1.FindByCapabilityResponse, error) {
    local := s.reg.FindByCapability(req.Capability)
    remote, _ := s.fanOutQuery(ctx, func(client discoveryv1.DiscoveryServiceClient) (*discoveryv1.FindByCapabilityResponse, error) {
        return client.FindByCapability(ctx, req)
    })
    return mergeResults(local, remote), nil
}
```

**Option B — Broadcast on registry change (v2):** When a module registers or
unregisters on any node, broadcast the capability index delta to all peers so
each node has a complete cluster-wide index. More complex but O(1) local
lookup.

Recommend Option A for v1 (small clusters), Option B for v2 (scale).

### 2.4 Files Changed

| File | Change |
|------|--------|
| `internal/grpcmesh/lb.go` | Add `RandomStrategy`, `LeastLoadedStrategy` |
| `pkg/contracts/cluster.go` | Add load metrics to `NodeInfo` if least-loaded strategy is used |
| `internal/grpcmesh/discovery_query.go` | Add fan-out aggregation `FindByCapability`, `FindByRole` |
| `internal/grpcmesh/discovery.go` | Optional: add peer-to-peer query RPC for aggregation |
| `cmd/muxcored/main.go` | Make LB strategy configurable via CLI flag |

---

## 3. Failover

### 3.1 Watchdog Improvements

Three issues with the current watchdog (`cmd/muxcore-watchdog/main.go:205`):

#### 3.1.1 TCP Dial → gRPC Health Probe

Replace `checkCore()` TCP dial with a proper gRPC health check:

```go
func (w *watchdog) checkCore(ctx context.Context) bool {
    conn, err := grpc.Dial(w.addrs[w.currentAddrIdx],
        grpc.WithTransportCredentials(insecure.NewCredentials()),
        grpc.WithBlock(), grpc.WithTimeout(2*time.Second))
    if err != nil {
        return false
    }
    defer conn.Close()
    health := healthpb.NewHealthClient(conn)
    resp, err := health.Check(ctx, &healthpb.HealthCheckRequest{Service: ""})
    return err == nil && resp.GetStatus() == healthpb.HealthCheckResponse_SERVING
}
```

This distinguishes "core is starting up" from "core is dead" and prevents
false-positive failovers during restarts.

#### 3.1.2 Exponential Backoff on Failover Errors

If `spawnModule()` fails after failover, the current code logs and retries on
the next tick (linear). Add backoff:

```go
type watchdog struct {
    // ...existing fields...
    failoverBackoff time.Duration
}
func (w *watchdog) failover() {
    // ...kill, rotate addr, spawn...
    if err != nil {
        w.failoverBackoff = min(w.failoverBackoff*2, 60*time.Second)
        time.Sleep(w.failoverBackoff)
    } else {
        w.failoverBackoff = 0
    }
}
```

#### 3.1.3 Watchdog Restartability

If the watchdog process itself crashes, core currently does not restart it.
Add a `watchProcess` loop for the watchdog similar to the module restart loop:

```go
// In internal/module/mgr/manager.go
func (m *Manager) spawnWatchdogWithRetry(ctx context.Context, bin ModuleBinary, addr []string) {
    for attempt := 0; attempt < 3; attempt++ {
        cmd := exec.CommandContext(ctx, m.watchdogPath,
            "--module-path", bin.Path,
            "--module-id", bin.ID,
            "--mesh-addrs", strings.Join(addrs, ","))
        if err := cmd.Start(); err != nil {
            // backoff and retry
            continue
        }
        if err := cmd.Wait(); err != nil {
            // watchdog crashed, retry
            continue
        }
        break
    }
}
```

#### 3.1.4 Flexible Module Argument Passing

The watchdog hardcodes `--muxcore-mesh-addr` as the flag to pass to the module.
Modules that use environment variables or config files for their mesh address
can't use the watchdog. Add a template-based argument specification:

```go
type SpawnConfig struct {
    // ArgsTemplate is a Go template executed with the chosen mesh address.
    // Default: ["--muxcore-mesh-addr", "{{.Addr}}"]
    ArgsTemplate []string

    // EnvTemplate is additional environment variables set before spawn.
    // Default: ["MUXCORE_MESH_ADDR={{.Addr}}"]
    EnvTemplate []string
}
```

---

## 4. Work Tracking & Redelivery

### 4.1 Current State

The WorkerPool (`internal/workerpool/pool.go:451`) is a complete task state
machine with persistence. It tracks tasks through their lifecycle and handles
reassignment on node departure. However, it has no mechanism to actually
**execute** tasks — it's a tracker, not a dispatcher.

### 4.2 Missing: Task Dispatcher / Executor Loop

The biggest gap. Add a `Dispatcher` that connects the WorkerPool to `Executor`
modules:

```go
// internal/workerpool/dispatcher.go

type Dispatcher struct {
    pool     *Pool
    registry contracts.Registry
    lb       contracts.LBStrategy
    logger   *slog.Logger
}

// DispatchLoop polls for Pending tasks and sends them to eligible executors.
func (d *Dispatcher) DispatchLoop(ctx context.Context, interval time.Duration) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            d.dispatchOnce(ctx)
        }
    }
}

func (d *Dispatcher) dispatchOnce(ctx context.Context) {
    tasks, _ := d.pool.List(ctx, &contracts.WorkerTaskFilter{Status: contracts.TaskStatusPending})
    for _, task := range tasks {
        // Find eligible executors for this task type
        candidates := d.registry.FindByCapability("executor." + task.Type)
        if len(candidates) == 0 {
            continue // no executor available for this task type
        }
        // Pick one using LB strategy
        idx, err := d.lb.Pick(ctx, candidatesToNodeInfo(candidates))
        if err != nil {
            continue
        }
        target := candidates[idx]
        // Dispatch via mesh call
        result, err := mesh.Call(ctx, target.Info.ID, "Execute", marshal(task))
        if err != nil {
            // Mark task as failed, will be retried
            d.pool.UpdateStatus(ctx, task.ID, contracts.TaskStatusFailed, err.Error())
            continue
        }
        _ = result
    }
}
```

The dispatcher runs as a goroutine alongside the pool. It uses the existing
`LBStrategy` to balance across executor instances and the existing mesh to
deliver the call.

### 4.3 Missing: IdempotencyProvider Implementation

The interface exists but has no implementation. For task redelivery, this is
critical:

```go
// internal/idempotency/store.go
// In-memory implementation backed by optional file persistence.

type MemoryIdempotencyStore struct {
    mu    sync.RWMutex
    store map[string]*idempotencyEntry
    file  string // optional: persist to JSON file
}

type idempotencyEntry struct {
    Key       string
    Result    []byte
    CreatedAt time.Time
    TTL       time.Duration
}
```

Needs a reaper goroutine to evict expired entries (prevent unbounded growth).
Integrate with the WorkerPool's reassignment flow: on `Reassign()`, the new
executor's call is gated by `IdempotencyProvider.Exists(task.IdempotencyKey)`.

### 4.4 Missing: DeadLetterProvider Implementation

Failed events and tasks need a dead letter queue for visibility and replay:

```go
// internal/deadletter/store.go

type FileDeadLetter struct {
    mu       sync.Mutex
    entries  []*contracts.DeadLetterEntry
    maxSize  int // max entries before rotation
    file     string
}

func (d *FileDeadLetter) Store(ctx context.Context, entry *contracts.DeadLetterEntry) error
func (d *FileDeadLetter) Replay(ctx context.Context, key string) error
func (d *FileDeadLetter) Discard(ctx context.Context, key string) error
```

Wire into the event bus handler path: when a subscriber's handler returns an
error, instead of just logging and dropping (current behavior in
`memory.go:394`), write to the dead letter store. Events in the dead letter
store are queryable and replayable via the task management API.

### 4.5 Missing: RetryProvider Implementation

Retry with backoff for mesh calls and event handlers:

```go
// internal/retry/retry.go

func Execute(ctx context.Context, policy contracts.RetryPolicy, fn func(context.Context) error) error {
    var err error
    for attempt := 0; attempt < policy.MaxAttempts; attempt++ {
        if err = fn(ctx); err == nil {
            return nil
        }
        if attempt < policy.MaxAttempts-1 {
            delay := calcBackoff(policy, attempt)
            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-time.After(delay):
            }
        }
    }
    return fmt.Errorf("all %d attempts failed: %w", policy.MaxAttempts, err)
}
```

### 4.6 Missing: Task Management API

HTTP/gRPC endpoints for operator visibility:

```
GET    /api/v1/tasks               — list all tasks, filterable by status/node/type
GET    /api/v1/tasks/:id           — single task detail
POST   /api/v1/tasks/:id/reassign  — manual reassignment to another node
POST   /api/v1/tasks/:id/cancel    — cancel a stuck task
POST   /api/v1/tasks/:id/retry     — retry a failed task
GET    /api/v1/deadletter          — list dead letter entries
POST   /api/v1/deadletter/:key/replay — replay a dead letter entry
GET    /api/v1/executors           — list registered executors and their task types
GET    /api/v1/executors/:id       — executor detail with load stats
```

### 4.7 Missing: EventStore Implementation

The `EventStore` interface (`pkg/contracts/eventstore.go:44-69`) has 4 methods —
`Append`, `Read`, `Subscribe`, `Streams` — but no provider. Implement it backed
by the existing WAL infrastructure:

```go
// internal/events/store.go

// WALEventStore wraps the WAL to implement contracts.EventStore.
type WALEventStore struct {
    wal *WALWriter
}

func (s *WALEventStore) Append(ctx context.Context, stream string, events []contracts.Event) (int64, error) {
    // Write events to WAL with stream tag, return next sequence number
}

func (s *WALEventStore) Read(ctx context.Context, stream string, fromSequence int64, limit int) ([]contracts.EventStoreEntry, error) {
    // Read from WAL segments, filter by stream
}

func (s *WALEventStore) Subscribe(ctx context.Context, stream string, fromSequence int64) (<-chan contracts.EventStoreEntry, error) {
    // Replay from WAL then live-subscribe to new events
}

func (s *WALEventStore) Streams(ctx context.Context) ([]string, error) {
    // Scan WAL segment headers for unique stream names
}
```

### 4.8 Missing: Graceful Drain for In-Flight Tasks

When core shuts down, in-flight tasks are silently lost. Add a drain phase:

```go
// internal/workerpool/pool.go

func (p *Pool) Shutdown(ctx context.Context) error {
    // 1. Stop accepting new tasks
    p.mu.Lock()
    p.accepting = false
    p.mu.Unlock()

    // 2. Wait for in-flight tasks to complete or timeout
    deadline, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()
    for {
        active, _ := p.List(deadline, &WorkerTaskFilter{Status: TaskStatusRunning})
        if len(active) == 0 {
            break
        }
        // Persist each running task as Pending for redelivery
        for _, t := range active {
            t.Status = TaskStatusPending
            t.AssignedNode = ""
            p.persistTask(deadline, t)
        }
        select {
        case <-deadline.Done():
            return deadline.Err()
        case <-time.After(500 * time.Millisecond):
        }
    }

    // 3. Stop reaper
    p.cancel()
    return nil
}
```

Wire into main.go's shutdown sequence at line 426-445, before bus.Close().

### 4.9 Files Changed (New Phase Group)

| File | Phase | Change |
|------|-------|--------|
| `internal/workerpool/dispatcher.go` | 10 | NEW — Task dispatcher / Executor loop |
| `internal/idempotency/store.go` | 12 | NEW — In-memory idempotency store |
| `internal/deadletter/store.go` | 11 | NEW — File-backed dead letter queue |
| `internal/retry/retry.go` | 11 | NEW — Retry with backoff, jitter |
| `internal/events/store.go` | 13 | NEW — WAL-backed EventStore |
| `internal/workerpool/pool.go` | 18 | ADD — `Shutdown()` with in-flight task drain |
| `pkg/contracts/worker.go` | 10 | ADD — `HeartbeatInterval` field on `WorkerTask` |
| `internal/api/` | 8 | NEW — Task management endpoints |
| `cmd/muxcored/main.go` | 8,18 | ADD — Wire APIs, shutdown drain |

---

## 5. Health Management

### 5.1 Current State

`HealthCheck()` exists (`internal/module/manager.go:143-155`) but is only called
on demand from the gRPC mesh health service. There is no periodic health check
loop and no automatic remediation when a module is unhealthy.

### 5.2 Missing: Health Check Scheduler

Add a ticker-driven health check loop with automatic restart:

```go
// internal/module/manager.go

func (m *Manager) StartHealthCheckLoop(ctx context.Context, interval time.Duration) {
    go func() {
        ticker := time.NewTicker(interval)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                m.healthCheckAndRemediate(ctx)
            }
        }
    }()
}

func (m *Manager) healthCheckAndRemediate(ctx context.Context) {
    unhealthy := m.HealthCheck(ctx) // existing method
    for moduleID, err := range unhealthy {
        if errors.Is(err, ErrProcessExited) {
            // Already handled by watchProcess, skip
            continue
        }
        // Module is alive but unhealthy — log, emit degraded event, restart
        slog.Warn("module unhealthy, triggering restart", "module", moduleID, "error", err)
        if restartErr := m.RestartModule(ctx, moduleID); restartErr != nil {
            slog.Error("failed to restart unhealthy module", "module", moduleID, "error", restartErr)
        }
    }
}
```

Default interval: 30 seconds. Configurable via CLI flag or config file.

### 5.3 Missing: RestartModule

`watchProcess` only handles process-exit-triggered restarts. Add a restart
method that works for health-triggered restarts:

```go
// internal/module/mgr/manager.go

func (m *Manager) RestartModule(ctx context.Context, moduleID string) error {
    m.mu.Lock()
    defer m.mu.Unlock()

    proc, ok := m.processes[moduleID]
    if !ok {
        return fmt.Errorf("module %q not running", moduleID)
    }

    // Send SIGINT, wait for exit
    if err := proc.Process.Signal(os.Interrupt); err != nil {
        // Force kill
        proc.Process.Kill()
    }
    proc.Wait()

    // Re-spawn with same binary
    bin := m.moduleBinaries[moduleID]
    cmd, err := m.spawnLocked(ctx, bin)
    if err != nil {
        return fmt.Errorf("restart failed: %w", err)
    }

    m.processes[moduleID] = cmd
    return nil
}
```

### 5.4 Missing: ClusterNodeDegraded Lifecycle

The `ClusterNodeDegraded` event type is defined in contracts but never emitted
and never handled. Add emission in `pollEvents()` and handling in main.go:

```go
// internal/grpcmesh/cluster.go — pollEvents()

// After computing current snapshot, compare health status of each node
for id, member := range current {
    prev, existed := prevMembers[id]
    if existed && hasNewDegradation(member, prev) {
        c.events <- contracts.ClusterEvent{
            Type: contracts.ClusterNodeDegraded,
            Node: protoNodeToContract(member),
        }
    }
}
```

```go
// cmd/muxcored/main.go — cluster event loop

case evt.Type == contracts.ClusterNodeDegraded:
    // Preemptively fail tasks on degraded nodes
    count := workerPool.FailNodeTasks(evt.Node.ID)
    // Don't resurrect yet — wait for actual NodeLeft
```

### 5.5 Missing: LeaderChanged Handler

Currently the event loop only handles `ClusterNodeLeft`. If the leader changes,
resurrection silently stops working until the next node departure:

```go
// cmd/muxcored/main.go — cluster event loop

case evt.Type == contracts.ClusterNodeLeaderChanged:
    // Re-check if this node is now leader and has pending orphan work
    if leader := cluster.Leader(); leader != nil && leader.ID == nodeID {
        // Check for any modules that were flagged as orphaned
        modMgr.ResurrectPendingOrphans(ctx)
    }
```

### 5.6 Missing: Registry State Machine Guards

`SetState` accepts any state transition. Add a valid transition table:

```go
// internal/registry/registry.go

var validTransitions = map[contracts.ModuleState][]contracts.ModuleState{
    contracts.ModuleStateRegistered: {contracts.ModuleStateStarting, contracts.ModuleStateDegraded},
    contracts.ModuleStateStarting:   {contracts.ModuleStateRunning, contracts.ModuleStateDegraded, contracts.ModuleStateStopping},
    contracts.ModuleStateRunning:    {contracts.ModuleStateDegraded, contracts.ModuleStateStopping},
    contracts.ModuleStateDegraded:   {contracts.ModuleStateRunning, contracts.ModuleStateStopping},
    contracts.ModuleStateStopping:   {contracts.ModuleStateStopped},
    contracts.ModuleStateStopped:    {},
}

func (r *Registry) SetState(id string, state contracts.ModuleState) error {
    r.mu.Lock()
    defer r.mu.Unlock()

    entry, ok := r.modules[id]
    if !ok {
        return fmt.Errorf("module %q not found", id)
    }

    allowed, ok := validTransitions[entry.State]
    if !ok {
        return fmt.Errorf("invalid transition from %q to %q", entry.State, state)
    }
    for _, s := range allowed {
        if s == state {
            entry.State = state
            return nil
        }
    }
    return fmt.Errorf("invalid transition from %q to %q", entry.State, state)
}
```

Also add `GetHealth(id string) (error, error)` convenience method returning
the health error and any lookup error, so callers don't have to `Get()` then
check `Entry.Health` directly.

### 5.7 Files Changed

| File | Change |
|------|--------|
| `internal/module/manager.go` | ADD `StartHealthCheckLoop()`, `healthCheckAndRemediate()` |
| `internal/module/mgr/manager.go` | ADD `RestartModule()` |
| `internal/grpcmesh/cluster.go` | ADD degradation detection in `pollEvents()`, emit `ClusterNodeDegraded` |
| `cmd/muxcored/main.go` | ADD `LeaderChanged`, `NodeDegraded` handlers; ADD health check loop start |
| `internal/registry/registry.go` | ADD state transition guards, `GetHealth()` convenience method |
| `pkg/contracts/module.go` | ADD `RestartModuleID` event type (optional) |

---

## 6. Circuit Breaking & Rate Limiting

### 6.1 Missing: Mesh Call Circuit Breaker

Currently, if a remote node's module starts failing, `HealthyCandidates()`
only removes it from consideration if `ModuleHealth` is non-empty. But
`ModuleHealth` is updated only on heartbeat (every 10s) — there's a window
where a degraded node keeps receiving traffic.

Add a circuit breaker to `Client.Call()`:

```go
// internal/grpcmesh/circuitbreaker.go

type CallCircuitBreaker struct {
    mu         sync.Mutex
    failures   map[string]*failureCount // nodeID → failures
    threshold  int                       // default: 5
    cooldown   time.Duration             // default: 30s
}

func (c *CallCircuitBreaker) RecordFailure(nodeID string)
func (c *CallCircuitBreaker) RecordSuccess(nodeID string)
func (c *CallCircuitBreaker) IsOpen(nodeID string) bool
```

Wire into `Client.Call()` after `routeToNode()`: if a call fails, increment
failure counter. If threshold exceeded, exclude that node from `HealthyCandidates`
for `cooldown` duration. On success, reset counter.

```go
// In Client.Call(), around line 428:
candidates = c.filterOpenCandidates(candidates)
// ... strategy.Pick ...
result, err := c.routeToNode(ctx, chosen, ...)
if err != nil {
    c.circuitBreaker.RecordFailure(chosen.ID)
} else {
    c.circuitBreaker.RecordSuccess(chosen.ID)
}
```

### 6.2 Files Changed

| File | Change |
|------|--------|
| `internal/grpcmesh/circuitbreaker.go` | NEW — per-node circuit breaker |
| `internal/grpcmesh/server.go` | Wire circuit breaker into `Client.Call()` |

---

## 7. Multi-Instance Module Support

### 7.1 Missing: Same Binary, Different IDs

Currently the sidecar manager derives module ID from the repo URL (unique).
To run two identical modules on one node (e.g., two instances to
split load), the module ID must be distinct while the binary is the same.

Add instance naming support to the spool/muxcore.json schema:

```json
{
  "modules": [
    {
      "repo": "https://github.com/Muxcore-Media/downloader-http",
      "version": "v1.0.0",
      "required": true,
      "instance_id": "qb-01",
      "config": {
        "port": 8082,
        "download_path": "/mnt/fast/downloads"
      }
    },
    {
      "repo": "https://github.com/Muxcore-Media/downloader-http",
      "version": "v1.0.0",
      "required": true,
      "instance_id": "qb-02",
      "config": {
        "port": 8083,
        "download_path": "/mnt/slow/downloads"
      }
    }
  ]
}
```

The derived module ID becomes `downloader-http-qb-01` instead of
`downloader-http`. The instance config is passed as environment
variables or CLI flags to the module binary.

### 7.2 Instance-Level LB Strategy

Once multi-instance exists, `LBStrategy.Pick()` needs to consider instances,
not just nodes. The current `NodeInfo` has `ModuleIDs []string` — that's one
entry per module ID per node. Multi-instance naturally produces multiple
entries per node, which the existing LB already handles (it picks from the
full candidate list).

However, instance-specific config means the caller might want a specific
instance rather than any. Add an optional `InstanceID` field:

```go
type CallOptions struct {
    TargetModule string
    TargetInstance string // optional: pin to a specific instance
    Timeout        time.Duration
    Headers        map[string]string
}
```

### 7.3 Files Changed

| File | Change |
|------|--------|
| `internal/module/mgr/manager.go` | Support `instance_id` in binary config, derive compound module ID |
| `pkg/contracts/mesh.go` | Add `TargetInstance` to mesh call options |

---

## 8. Module Compatibility — What Requires Module Changes

### 8.1 Zero Module Changes Needed

| Feature | Why |
|---------|-----|
| **Round-robin LB** | `ModuleMeshClient.Call()` routes transparently. Callers and callees are unaware of the distribution strategy. |
| **Health-aware routing** | Process-liveness is the default. A module that's running gets traffic. No new interfaces to implement. |
| **Watchdog (core-spawned)** | Transparent OS-level wrapper. The module binary doesn't know it's being monitored. |
| **Cluster resurrection** | Core decides to re-spawn the module on another node. The module starts fresh as if first launch. |
| **Cross-cluster FindByCapability** | Module discovery works the same way — aggregation happens on the server side. |
| **Circuit breaker** | Core tracks call failures per node. Modules are unaware. |
| **Health check scheduler** | Core calls `Health()` and decides to restart. Module just implements `Health()` as before. |
| **State machine guards** | Registry change only. Module behavior is unaffected. |

### 8.2 Minimal Init Change

| Feature | What the module must change |
|---------|----------------------------|
| **Connection failover** | The module must accept `--muxcore-mesh-addrs` (comma-separated, plural) instead of a single `--muxcore-mesh-addr`, or use `DialWithAddrs()` in the SDK. The SDK auto-reconnects on connection loss. |
| **Multi-instance** | The module should accept instance-specific config via env vars or CLI flags (e.g., `--instance-id`, `MUXCORE_INSTANCE_CONFIG_PATH`). |

Flag parsing change only — no business logic modification.

### 8.3 Real Module Code Changes

| Feature | What the module must change |
|---------|----------------------------|
| **Work redelivery** | The module must implement `contracts.Executor` (`CanHandle(taskType)`, `Execute(ctx, task)`) and call `WorkerPool.Heartbeat(taskID)` periodically during processing. Without this, the pool has nothing to track or reassign. |
| **Work redelivery (idempotency)** | The module should check `IdempotencyProvider.Exists(key)` before executing and return cached result if present. For downloads with side effects, this prevents duplicate work. |
| **Application health** | The module already implements `Health() error`. For health-triggered restart to work well, `Health()` should return meaningful errors (not nil when the module is struggling). |

### 8.4 Summary Table

```
┌─────────────────────────────────┬──────────┬──────────────────────────────────┐
│ Feature                         │ Module   │ What changes                     │
│                                 │ work     │                                  │
├─────────────────────────────────┼──────────┼──────────────────────────────────┤
│ Round-robin calls               │ None     │ —                                │
│ Health-aware routing            │ None     │ —                                │
│ Watchdog                        │ None     │ —                                │
│ Resurrection                    │ None     │ —                                │
│ Cross-cluster discovery         │ None     │ —                                │
│ Circuit breaker                 │ None     │ —                                │
│ Health check scheduler          │ None     │ — (but better Health() helps)    │
│ State machine guards            │ None     │ —                                │
├─────────────────────────────────┼──────────┼──────────────────────────────────┤
│ Connection failover             │ Minimal  │ Init/flag change only            │
│ Multi-instance support          │ Minimal  │ Accept instance-specific config  │
├─────────────────────────────────┼──────────┼──────────────────────────────────┤
│ Work redelivery                 │ Required │ Implement Executor + hb loop     │
│ Idempotency                     │ Required │ Check IdempotencyProvider       │
└─────────────────────────────────┴──────────┴──────────────────────────────────┘
```

---

## 9. Interaction Summary

```
                          ┌─────────────────────────────────────────────────────┐
                          │              ModuleMeshClient                       │
                          │  ┌──────────┐  ┌────────────┐  ┌──────────────┐   │
                          │  │ Round-   │  │ Circuit   │  │ Healthy-     │   │
                          │  │ Robin LB │  │ Breaker   │  │ Candidates   │   │
                          │  └──────────┘  └────────────┘  └──────────────┘   │
                          └──────────────────────┬──────────────────────────────┘
                                                 │
            ┌────────────────────────────────────┼────────────────────────────────────┐
            ▼                                    ▼                                    ▼
      ┌──────────┐                         ┌──────────┐                         ┌──────────┐
      │ Core A   │                         │ Core B   │                         │ Core C   │
      │ download │                         │ download │                         │ download │
      │ er       │                         │ er       │                         │ er       │
      └────┬─────┘                         └────┬─────┘                         └────┬─────┘
           │                                    │                                    │
           │ WorkerPool + Dispatcher            │ WorkerPool                         │ Dispatcher
           │ assigns tasks                      │ detects node loss                  │ picks eligible
           │ monitors heartbeats                │ reassigns tasks                    │ executor
           ▼                                    ▼                                    ▼
      ┌─────────────────────────────────────────────────────────────────────────────────┐
      │                           WorkerPool (distributed)                               │
      │  tracks: taskID → assignedNode, status, idempotencyKey, heartbeats              │
      │  persists: FileStore (JSON per task)                                             │
      │  dispatches: Polls Pending, finds "executor.<type>" via registry, routes via LB │
      └─────────────────────────────────────────────────────────────────────────────────┘
                            ▲                         ▲
                            │                         │
                    ┌──────────────┐          ┌──────────────┐
                    │ Idempotency  │          │ DeadLetter   │
                    │ Provider     │          │ Store        │
                    │ (dedup check)│          │ (failed task │
                    │              │          │  replay)     │
                    └──────────────┘          └──────────────┘
```

---

## 10. Implementation Order

| Phase | Scope | Dependencies | Status |
|-------|-------|--------------|--------|
| 1 | Round-robin LB in `Client.Call()` | None | ✅ Done |
| 2 | Health-aware routing (ModuleHealth in heartbeats) | Phase 1 | ✅ Done |
| 3 | SDK reconnection + multi-address support | None (independent) | ✅ Done |
| 4 | Watchdog process for core-spawned modules | Phase 3 | ✅ Done |
| 5 | WorkerPool in-memory implementation | Phase 2 (for task-to-node routing) | ✅ Done |
| 6 | Task heartbeat + reaper + NodeLeft reassignment | Phase 5 | ✅ Done |
| 7 | Storage-backed WorkerPool persistence | Phase 5 | ✅ Done |
| 8 | **Task management API endpoints** | Phase 5 | ✅ Done | `internal/api/tasks.go` |
| 9 | Module resurrection on NodeLeft | Phase 3 | ✅ Done |
| 10 | **Task dispatcher / Executor loop** | Phase 5, 6 | ✅ Done | `internal/workerpool/dispatcher.go` |
| 11 | **Dead letter + retry provider implementations** | Phase 5 | ✅ Done | `internal/deadletter/store.go`, `internal/retry/retry.go` |
| 12 | **Idempotency provider implementation** | Phase 10 | ✅ Done | `internal/idempotency/store.go` |
| 13 | **EventStore implementation (WAL-backed)** | Phase 5 | ✅ Done | `internal/eventstore/store.go` (not `internal/events/store.go`) |
| 14 | **Health check scheduler + auto-restart** | Phase 2, 4 | ✅ Done | `internal/module/manager.go` |
| 15 | **ClusterNodeDegraded lifecycle (emit + handle)** | Phase 14 | ✅ Done | `cluster.go:193`, `main.go:265` |
| 16 | **LeaderChanged handler** | Phase 9 | ✅ Done | `main.go:275` |
| 17 | **Watchdog gRPC health probe + exponential backoff** | Phase 4 | ✅ Done | `cmd/muxcore-watchdog/main.go` |
| 18 | **Worker pool graceful drain on shutdown** | Phase 5 | ✅ Done | `pool.go:117-145` — `Shutdown()` |
| 19 | **Cross-cluster discovery aggregation (Option A)** | Phase 2 | ✅ Done | `discovery_query.go` — `fanOutQuery()` |
| 20 | **Registry state machine guards + GetHealth()** | — | ✅ Done | `registry.go:171-224` |
| 21 | **Mesh call circuit breaker** | Phase 2 | ✅ Done | `internal/grpcmesh/circuitbreaker.go` |
| 22 | **Multi-instance module spawn + instance config** | Phase 4 | ✅ Done | `mgr/manager.go` — `InstanceID`, `resolveWithInstance()` |
| 23 | **Extra LB strategies (Random, LeastLoaded)** | Phase 1 | ✅ Done | `lb.go:33-99` |
| 24 | **Cross-cluster discovery aggregation (Option B)** | Phase 19 | ❌ Not implemented | Explicitly deferred to v2 in the design; Option A (`fanOutQuery`) is sufficient for v1 |

---

## 11. Known Edge Cases & Failure Modes

These are failure conditions that the design must handle correctly:

### 11.1 Split-brain on leader election

Current leader election is deterministic (lowest node ID wins). If a network
partition isolates a subset of nodes, both partitions elect their own leader.
Both leaders may attempt to resurrect modules and reassign tasks to nodes
they can't actually reach.

**Mitigation:** Leader actions (resurrection, reassignment) require a quorum
check before executing. If the leader can't reach >50% of members, it stops
taking leader-like actions. Add a quorum check to `ResurrectOrphan` and
`FailNodeTasks` paths.

### 11.2 Double resurrection

If the heartbeat interval (10s) is shorter than the eviction timeout (30s),
a node that's slow but not dead could be evicted, have its modules resurrected
elsewhere, then rejoin — resulting in two copies of the same module.

**Mitigation:** Add a probation period after eviction. Before resurrecting, check
if the departed node has been gone longer than `evictionTimeout + gracePeriod`
(45s total). Also, on rejoin, the rejoining node should be told to kill modules
that were resurrected elsewhere.

### 11.3 Task replay thrashing

If a task has a tight retry budget and the executor is consistently crashing,
wasteful thrashing occurs: submit → assign → crash → reassign → crash.

**Mitigation:** Add a per-task-type failure rate limit in the dispatcher. If
executors for `executor.download` have a >50% failure rate in the last 5
minutes, stop dispatching new download tasks and emit a warning event.

### 11.4 Idempotency key collision

Two unrelated tasks could theoretically produce the same idempotency key if the
generation function is poor.

**Mitigation:** Idempotency keys should be `{taskType}:{sourceID}:{UUID}` or
similar scoped composite. Document the key generation convention in the
contracts package.

### 11.5 Watchdog cascade failure

If all cores in the cluster fail simultaneously, every watchdog kills its
module and cycles through addresses at roughly the same rate. When the first
core comes back, all modules try to re-register at once.

**Mitigation:** Add jitter to the watchdog's failover backoff:
`baseDelay + rand.Float64() * jitterWindow`. Modules reconnect at staggered
intervals.

### 11.6 Stale FileStore after cluster topology change

If the persisted task directory is on shared storage (NFS, etc.), two nodes
could load the same tasks on startup, duplicating work.

**Mitigation:** `FileStore` should be local-only. A shared-file `TaskStore`
implementation is a future option but must use file locking. The v1 assumption
is that each node's task store is private; tasks are only reassigned via
cluster events, not by second-loading from disk.

### 11.7 Executor module disappears mid-dispatch

The dispatcher selects an executor via `FindByCapability()`, but by the time
the mesh call arrives, that module may have unregistered or crashed.

**Mitigation:** The dispatcher already handles the error path — it marks the
task as Failed and lets the reaper/reassign loop pick it up. The reaper should
have a short retry delay (a few seconds) for tasks that failed with
`ErrRemoteRoutingUnavailable` since the executor might just be restarting.

---

## 12. Observability

Every resilience feature should produce visible signals. Minimum observability
requirements per phase:

| Phase | Metrics | Logs | Events |
|-------|---------|------|--------|
| LB | Calls per node, LB strategy distribution | Routing decisions | — |
| Circuit breaker | Open/closed count per node, failure rate | Circuit open/close transitions | — |
| Watchdog | Failover count per module, reconnect latency | Address cycling, failover triggers | — |
| Resurrection | Adoptions per leader term, spawn failures | Resurrection failures | `module.resurrected` |
| WorkerPool | Pending/Running/Failed tasks per type, queue depth | Task state transitions | `task.submitted`, `task.completed`, `task.failed` |
| Dispatcher | Dispatch latency, executor availability | Dispatch failures | — |
| Health check | Healthy/unhealthy modules per type, restart count | Health degradation, restarts | `module.degraded`, `module.restarted` |
| Dead letter | Entries per handler, replay count | Dead letter stores | — |
| Idempotency | Cache hit/miss rate, entry count | Duplicate detection | — |

Add a Prometheus metrics endpoint to the API server exposing counters and
gauges for each row above.
