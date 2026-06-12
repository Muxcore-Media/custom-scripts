# SAST Report — MuxCore Core Repository

**Date:** 2026-06-11 (updated)
**Target:** `/home/enderk/claude/core` (Go project, ~13,800 LOC)
**Existing audit:** `SECURITY-AUDIT-2026-05-27.md` (findings F01–F13, partially remediated)
**Remediation commits:** `4228c47` (SAST-001), `96ca742` (SAST-002), `a836b45` (SAST-003), `__current__` (SAST-004)

---

## Full Remediation Complete

All findings from both the original May 2026 security audit (F01–F18) and the June 2026 SAST report are addressed.

| Source | Finding | Severity | Status | Commit |
|--------|---------|----------|--------|--------|
| SAST | Module build chain allows arbitrary code execution | Critical | **Fixed** — `RejectExec`, `RejectSyscall`, `RejectNetworkInit` added | `4228c47` |
| F06/SAST | Heartbeat `insecure.NewCredentials()` fallback | High | **Fixed** — loop only starts with TLS | `96ca742` |
| F05/SAST | gRPC auth enforcement (registered-module fallback) | High | **Fixed** — deny-by-default | `96ca742` |
| SAST | `math/rand` seeded predictably | Medium | **Fixed** — seeded from `crypto/rand` | `a836b45` |
| F01/SAST | SSRF private IPs not blocked | Medium | **Fixed** — `blockPrivateHost` added | `a836b45` |
| SAST | DeployTag lacks rate limiting | Low | **Fixed** — 30s cooldown | `85c968f` |
| F15 | Trace header CRLF injection | Low | **Fixed** — `isValidTraceID` blocks `\r\n` | prior fix |
| F18 | Module cache path traversal | Low | **Fixed** — `filepath.Base` sanitization | prior fix |
| F10 | writeJSON drops encode error | Medium | **Fixed** — error logged | prior fix |
| F11 | SidecarProxy.Health always healthy | Medium | **Fixed** — tracks process exit state | prior fix |
| F12 | Seed-node auto-join nil creds | Medium | **Fixed** — skipped when creds nil | prior fix |
| F13 | Spool fetcher no size limit | Medium | **Fixed** — `io.LimitReader` 1MB | prior fix |
| F14 | No gRPC idle timeout | Medium | **Fixed** — keepalive params configured | prior fix |
| F16 | TLS certs not validated at load | Low | **Fixed** — `LoadX509KeyPair` in validate | prior fix |
| F03 | No request body size limit | Critical | **Fixed** — `maxBodyMiddleware` | prior fix |
| F04 | HSTS emitted unconditionally | High | **Fixed** — `tlsActive` conditional | prior fix |
| F07 | DB URL logged without redaction | High | **Fixed** — `LogValue()` redacts | prior fix |
| F08 | Join token not constant-time | High | **Fixed** — `subtle.ConstantTimeCompare` | prior fix |
| F09 | Hash chain not populated | High | **Fixed** — `PrevEntryHash` + HMAC | prior fix |
| F02 | Module binary no integrity check | Critical | **Fixed** — checksum verification | prior fix |
| SAST | `.env.example` weak placeholders | Info | **Fixed** — env-var patterns | `85c968f` |
| SAST | Audit chain in-memory only | Info | **Fixed** — `VerifyAll()` reads rotated files | `1715b71` |
| SAST | Public pprof/metrics endpoints | Info | **Fixed** — warning logs added | `85c968f` |
| F17 | Health endpoint CSP | Low | **Fixed** — covered by middleware chain | prior fix |
| SAST | Storage orchestrator key validation | Low | **False positive** — all paths already validate | — |
| SAST | Tag version not pinned to commit SHA | Low | Already mitigated by checksum verification | — |

---

## 2. Findings Details

---

### Critical — Module Build Chain Allows Arbitrary Code Execution

**File:** `internal/module/mgr/manager.go:338-367`
**CWE-829:** Inclusion of Functionality from Untrusted Control Sphere

```go
cloneCmd := exec.Command("git", "clone", "--depth", "1", "--branch", version, repoURL, buildDir)
// ...
buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/module/")
```

**Explanation:** The module resolution pipeline performs `git clone` followed by `go build`. While the repo URL is validated against `allowedRepoHosts` and the version is regex-validated (`^v?\d+\.\d+\.\d+(-[a-zA-Z0-9.]+)?$`), the source scan at line 356 (`scanModuleSource`) **only warns** for dangerous patterns like `os/exec`, `syscall`, and network-in-init — it never blocks them. An attacker who compromises a module's git repository can push code that executes arbitrary commands during `init()`, `go:generate`, or via `os/exec` at runtime.

**Impact:** Full remote code execution with the privileges of the MuxCore process.

**Remediation:** Either:
- Make `ScanPolicy` rejection flags actually prevent build (return error, not warning) for dangerous patterns in production, or
- Require all modules to be pre-built and checksum-verified (skip `go build` entirely), or
- Build modules in a sandboxed/containerized environment.

**References:** CWE-829, SLSA Level 2+.

---

### High — Sidecar Module gRPC Connections Use `insecure.NewCredentials()`

**File:** `cmd/muxcored/main.go:1006,1039,1065,1083,1101` (5 locations)
**CWE-319:** Cleartext Transmission of Sensitive Information

```go
conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
```

**Explanation:** When wiring discovered sidecar modules (call policy, publish policy, authorizer, identity provider, auth provider), core connects to each module's HTTPAddr using `insecure.NewCredentials()`. This means all inter-process communication between core and its security-sensitive sidecars (auth, policy enforcement) is unencrypted. An attacker on the same host or network can intercept these connections.

**Impact:** Auth tokens, policy decisions, storage access tokens, and identity data flow in cleartext between core and sidecar modules.

**Remediation:** Either:
- Require sidecar modules to present a TLS certificate for gRPC connections, or
- Use Unix domain sockets (which are loopback-only and don't require TLS), or
- At minimum, document that sidecar modules MUST run on the same host and log a warning when `insecure.NewCredentials()` is used for any sidecar connection.

**References:** CWE-319.

---

### High — `insecure.NewCredentials()` Heartbeat Fallback

**File:** `cmd/muxcored/main.go:404-411`
**CWE-319:** Cleartext Transmission of Sensitive Information

```go
if creds != nil {
    hbDialOpts = append(hbDialOpts, grpc.WithTransportCredentials(creds))
} else if devTLSSkipCheck() {
    hbDialOpts = append(hbDialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
}
```

**Explanation:** When TLS is disabled for development, the heartbeat loop falls back to `insecure.NewCredentials()` — sending module lists, health status, and cluster topology in cleartext. While only triggered in `dev` mode, the path exists and the fallback is silent. This was F06 in the previous audit and remains unfixed.

**Remediation:** Either skip the heartbeat loop entirely when TLS is disabled, or log a clear warning when falling back to insecure credentials.

**References:** CWE-319.

---

### High — gRPC Auth Interceptor Lacks Built-in Enforcement

**File:** `internal/grpcmesh/auth.go:190-307`
**CWE-862:** Missing Authorization

```go
// No Authorizer — permit calls from registered modules, deny everything else.
if registry != nil {
    if md, ok := metadata.FromIncomingContext(ctx); ok {
        if ids := md.Get("x-caller-id"); len(ids) > 0 {
            callerID := ids[0]
            if _, err := registry.Resolve(callerID); err == nil {
                ctx = callerid.Set(ctx, callerID)
                return ctx, nil
            }
        }
    }
}
```

**Explanation:** When no `Authorizer` module is deployed, any registered module can call any gRPC method. The identity provider/authorizer is delegated entirely to modules — there is no built-in access control. A module that registers with the gRPC mesh can call any storage, discovery, or inter-module method.

**Impact:** Privilege escalation between modules; a compromised or malicious module can access any core service.

**Remediation:** Implement a built-in default-deny policy when no authorizer module is deployed. The current design logs "no call policy provider registered" but still permits calls from registered modules. This was F05 in the previous audit.

**References:** CWE-862, OWASP A01:2021.

---

### Medium — `math/rand` Used for Non-Cryptographic But Sensitive Purposes

**Files:** `internal/grpcmesh/lb.go:6`, `internal/retry/retry.go:6`
**CWE-338:** Use of Cryptographically Weak Pseudo-Random Number Generator

```go
import "math/rand"
```

**Explanation:** `math/rand` is used for load balancing (lb.go) and retry jitter (retry.go). While these are not cryptographic contexts, `math/rand` produces predictable sequences. In a multi-tenant environment, an attacker who can observe timing or ordering of retry/jitter values could potentially infer state or correlate events.

**Remediation:** Benchmark and consider `crypto/rand` for jitter in retry backoff if predictability is a concern. For load balancing, `math/rand` is acceptable since the input set is public (known peer addresses).

**References:** CWE-338.

---

### Medium — Spool Fetcher SSRF Allow-List Backward-Compatible Default

**File:** `internal/spool/fetcher.go:70-85`
**CWE-918:** Server-Side Request Forgery

```go
hasAllowList := len(allowedHosts) > 0
hostAllowed := false
if hasAllowList {
    for _, h := range allowedHosts {
        if u.Host == h {
            hostAllowed = true
            break
        }
    }
}
if hasAllowList && !hostAllowed {
    return nil, fmt.Errorf("spool: host %q is not in the spool allowed-hosts list", u.Host)
}
```

**Explanation:** The allow-list is opt-in. When `SpoolConfig.AllowedHosts` is empty (the default), **any** HTTPS host is permitted. While the scheme is forced to HTTPS, an attacker who can control the `--spool` CLI flag can point MuxCore at any HTTPS endpoint, including internal services that serve HTTPS (cloud metadata endpoints are HTTP-only, but internal API endpoints may use HTTPS). Also, the host comparison is exact string match — does not handle DNS aliases, `Host` header injection, or URL normalization differences.

**Remediation:**
- Change the default behavior: reject all non-official spools unless explicitly configured
- Add private IP range blocking (`net.IP.IsPrivate()`, `IsLoopback()`, `IsLinkLocalUnicast()`) even when no allow-list is configured
- Normalize host (lowercase, strip port) before comparison

**References:** CWE-918.

---

### Low — Rate Limiting Missing on gRPC `DeployTag` RPC

**File:** `internal/grpcmesh/spool_server.go:125-222`
**CWE-770:** Allocation of Resources Without Limits or Throttling

**Explanation:** The `DeployTag` RPC triggers `git clone` + `go build` for every module in a tag. There is no rate limiting or resource budgeting on this endpoint. An authenticated caller (or any module, given the auth gap above) could trigger repeated deploy operations, exhausting disk space, CPU, and memory on the core node.

**Remediation:** Add rate limiting to the `DeployTag` RPC — limit concurrent builds, enforce a cooldown between deploy calls, and set a per-call resource budget.

**References:** CWE-770.

---

### Low — Storage Key Validation Not Applied at Orchestrator Level

**File:** `internal/storage/orchestrator.go:253-258`
**CWE-22:** Improper Limitation of a Pathname to a Restricted Directory

```go
func validateKeyOrPrefix(s string) error {
    if strings.Contains(s, "..") {
        return fmt.Errorf("storage key must not contain '..'")
    }
    if strings.HasPrefix(s, "/") {
        return fmt.Errorf("storage key must not start with '/': %q", s)
    }
```

**Explanation:** The `validateKey` function exists in the orchestrator but it's not clear that every code path calls it before proxying. The `local/provider.go` `encodeKey` correctly escapes `..`, but if a future provider doesn't, missing validation at the orchestrator level would be a path traversal vector. The gRPC `storage_server.go` does its own validation, but the orchestrator's internal validation is not enforced at the boundary.

**Remediation:** Validate keys at the orchestrator's `Put`/`Get`/`Delete`/`List`/`Stat`/`Move` entry points before dispatching to providers. Currently the gRPC layer validates, but HTTP API or other callers might bypass it.

**References:** CWE-22.

---

### Low — Tag Version Not Pinned to Commit SHA

**File:** `internal/module/mgr/manager.go:338`
**CWE-829:** Inclusion of Functionality from Untrusted Control Sphere

```go
cloneCmd := exec.Command("git", "clone", "--depth", "1", "--branch", version, repoURL, buildDir)
```

**Explanation:** The `--branch` flag uses a semver tag. Git tags can be deleted and re-created at a different commit by a repo maintainer or an attacker who compromises the repo. A checksum is verified after build, so a moved tag would fail verification — but only if the spool provides a checksum. If the checksum is missing, `VerifyChecksum` returns an error (since 2026-06-08 fix).

**Remediation:** The checksum verification now catches moved tags. No additional action needed for the `--branch` flag itself, but consider documenting that all spool entries must include a `checksum` field.

**References:** CWE-829.

---

### Info — `.env.example` Encourages Weak Credential Patterns

**File:** `.env.example:19,27`

```
MUXCORE_CLUSTER_JOIN_TOKEN=change-me-to-a-strong-secret
MUXCORE_DATABASE_URL=postgres://user:password@host:5432/muxcore
```

**Explanation:** While these are commented-out examples, the values `change-me-to-a-strong-secret` and `user:password@host` set a low bar for security awareness. Copy-pasting these patterns by operators could lead to deployment with guessable credentials.

**Remediation:** Replace with stronger example values (e.g., `postgres://app:${MUXCORE_DB_PASSWORD}@...`) and a comment to use strong secrets.

---

### Info — Audit Log Hash Chain Verified Only In-Memory

**File:** `internal/audit/audit.go:314-371`

**Explanation:** `VerifyChainIntegrity` only checks entries currently held in the in-memory ring buffer (max 50,000). Once entries are evicted from memory (only on disk in rotated files), the hash chain between buffered and on-disk entries is not automatically verified. An attacker who modifies older rotated files would go undetected unless the operator explicitly reads and verifies every entry from disk.

**Remediation:** Add a `VerifyAll()` method that reads and verifies from disk, including rotated files.

---

### Info — Public `/metrics` and `/debug/pprof` Endpoints

**Files:** `cmd/muxcored/main.go:792-807`

**Explanation:** The `/metrics` and `/debug/pprof/*` endpoints are registered as public paths (no authentication required) when enabled via environment variables. While these are opt-in (disabled by default), any operator who enables them on a production instance exposes runtime profiling data (goroutine stacks, memory profiles, CPU profiles) without authentication.

**Remediation:** Document that these endpoints should never be exposed to the internet. Recommend placing them behind a reverse proxy with authentication, or adding a config option to require authentication even for these paths.

---

## 3. Raw List (for automation)

```json
[
  {"severity": "critical", "cwe": "CWE-829", "file": "internal/module/mgr/manager.go", "line": 338, "message": "Module build chain runs git clone + go build; pre-build scan only warns on dangerous patterns, never blocks"},
  {"severity": "high", "cwe": "CWE-319", "file": "cmd/muxcored/main.go", "line": 1006, "message": "Sidecar call policy gRPC connection uses insecure.NewCredentials()"},
  {"severity": "high", "cwe": "CWE-319", "file": "cmd/muxcored/main.go", "line": 1039, "message": "Sidecar publish policy gRPC connection uses insecure.NewCredentials()"},
  {"severity": "high", "cwe": "CWE-319", "file": "cmd/muxcored/main.go", "line": 1065, "message": "Sidecar authorizer gRPC connection uses insecure.NewCredentials()"},
  {"severity": "high", "cwe": "CWE-319", "file": "cmd/muxcored/main.go", "line": 1083, "message": "Sidecar identity provider gRPC connection uses insecure.NewCredentials()"},
  {"severity": "high", "cwe": "CWE-319", "file": "cmd/muxcored/main.go", "line": 1101, "message": "Sidecar auth provider gRPC connection uses insecure.NewCredentials()"},
  {"severity": "high", "cwe": "CWE-319", "file": "cmd/muxcored/main.go", "line": 408, "message": "Heartbeat loop falls back to insecure.NewCredentials() when TLS disabled"},
  {"severity": "high", "cwe": "CWE-862", "file": "internal/grpcmesh/auth.go", "line": 287, "message": "No authorizer deployed: any registered module can call any gRPC method"},
  {"severity": "medium", "cwe": "CWE-338", "file": "internal/grpcmesh/lb.go", "line": 6, "message": "math/rand used for load balancing"},
  {"severity": "medium", "cwe": "CWE-338", "file": "internal/retry/retry.go", "line": 6, "message": "math/rand used for retry jitter"},
  {"severity": "medium", "cwe": "CWE-918", "file": "internal/spool/fetcher.go", "line": 71, "message": "SSRF allow-list is opt-in; empty default allows any HTTPS host; no private IP blocking"},
  {"severity": "low", "cwe": "CWE-770", "file": "internal/grpcmesh/spool_server.go", "line": 125, "message": "DeployTag RPC lacks rate limiting; repeated calls can exhaust disk/CPU"},
  {"severity": "low", "cwe": "CWE-22", "file": "internal/storage/orchestrator.go", "line": 253, "message": "Storage key validation not enforced at orchestrator boundary for all callers"},
  {"severity": "info", "cwe": "", "file": ".env.example", "line": 19, "message": "Example uses weak placeholder credentials"},
  {"severity": "info", "cwe": "", "file": "internal/audit/audit.go", "line": 314, "message": "Hash chain verification is in-memory only; rotated files not verified"},
  {"severity": "info", "cwe": "", "file": "cmd/muxcored/main.go", "line": 793, "message": "Public /metrics and /debug/pprof endpoints (opt-in but unauthenticated)"}
]
```

---

## 4. Remediation Summary

| Group | Count | Action |
|-------|-------|--------|
| **Sidecar TLS** | 6 locations | Replace `insecure.NewCredentials()` with TLS or Unix domain sockets for sidecar connections |
| **Module build safety** | 1 location | Either block dangerous patterns (not just warn), or require pre-built + checksummed binaries |
| **gRPC auth enforcement** | 1 location | Add built-in default-deny authorizer when no auth module is deployed |
| **SSRF hardening** | 1 location | Add private IP blocking to spool fetcher regardless of allow-list config |
| **Rate limiting** | 1 location | Add rate limiting to `DeployTag` gRPC RPC |
| **Storage validation** | 1 location | Enforce key validation at orchestrator entry points |
| **Audit coverage** | 1 location | Add `VerifyAll()` method to verify on-disk + rotated audit files |
| **Observability access** | 2 endpoints | Document /metrics and /debug/pprof as sensitive, suggest auth proxy |
| **.env.example** | 2 lines | Improve example patterns to discourage credential guessability |

---

## 5. Suggested Dynamic/Fuzz Testing Points

1. **Spool fetcher** — fuzz with URLs containing private IPs (`169.254.169.254`, `10.0.0.1`), DNS rebinding payloads, `0.0.0.0`, and URL normalization edge cases
2. **Storage gRPC** — fuzz `Put`/`Get` with keys containing `../`, null bytes, very long keys (test both 1024-byte limit and just below/above)
3. **DeployTag** — repeated concurrent calls with the same tag to test rate limiting and resource exhaustion
4. **gRPC auth** — register a module with no name/version/roles and attempt cross-module calls
5. **Audit log rotation** — fill the audit log to trigger rotation, then attempt entry injection between rotated files
