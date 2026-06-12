# Security Audit — Phase 0: Tooling Baseline

Date: 2026-06-11
Auditor: Automated tooling

## Tools Used

| Tool | Version | Scope |
|------|---------|-------|
| `gosec` | v2.27.1 | `./internal/... ./pkg/... ./cmd/...` (49 files, 14530 LOC) |
| `govulncheck` | v1.3.0 | `./internal/... ./pkg/... ./cmd/...` |
| `golangci-lint` | v2.12.2 | `./internal/... ./pkg/... ./cmd/...` (enhanced config) |
| `go vet` | go1.26.4 | Same scope |
| `staticcheck` | via golangci-lint | Same scope |

## 1. govulncheck — Dependency CVEs

**Result: No vulnerabilities found.**

All dependencies are current; `golang.org/x/net v0.55.0` is past all known CVE fix points (CVE-2026-27141, CVE-2026-33814, CVE-2026-42502/42506/25680/25681/27136 all fixed in earlier versions).

## 2. gosec — Static Security Analysis

**83 total findings** (14 HIGH, 16 MEDIUM, 53 LOW)

### HIGH (14)

| Rule | Location | Details |
|------|----------|---------|
| G115 | `internal/grpcmesh/lb.go:30` | Integer overflow conversion uint64→int |
| G115 | `internal/grpcmesh/storage_server.go:329` | Integer overflow conversion rune→byte |
| G115 | `internal/grpcmesh/events.go:230` | Integer overflow conversion int64→uint64 |
| G115 | `internal/grpcmesh/spool_server.go:217` | Integer overflow conversion int→int32 |
| G115 | `internal/grpcmesh/lifecycle_server.go:87` | Integer overflow conversion int→int32 |
| G115 | `internal/grpcmesh/audit_server.go:52,138` | Integer overflow conversion int→int32 (2x) |
| G404 | `internal/retry/retry.go:70` | Weak random (math/rand instead of crypto/rand) |
| G404 | `internal/grpcmesh/lb.go:41` | Weak random for load balancing (math/rand) |
| G703 | `internal/module/mgr/manager.go:385` | Path traversal via taint analysis |
| G118 | `internal/grpcmesh/server.go:118` | Goroutine uses context.Background with request context available |
| G118 | `internal/events/memory.go:309` | Same pattern |
| G118 | `cmd/muxcored/main.go:1153` | Same pattern |

### MEDIUM (16)

| Rule | Count | Location Pattern | Details |
|------|-------|-----------------|---------|
| G204 | 4 | `module/mgr/manager.go:343,372,85` + `watchdog/main.go` | Subprocess launched with variable (git clone, go build) |
| G304 | 12 | Various | Potential file inclusion via variable |

### LOW (53)

G104 — Errors unhandled across provider.go, checks.go, manager.go, audit.go, server.go, main.go, etc.

## 3. golangci-lint — Enhanced Config

**151 issues across 7 linter categories**

| Linter | Count | Severity | Notes |
|--------|-------|----------|-------|
| govet | 49 | Low-Med | Mostly fieldalignment (43); shadow declarations (2); |
| errcheck | 46 | Low-Med | Unchecked error returns — many are benign (Close, Write in HTTP handlers) but some are security-relevant (Unregister, SetState, SetHealth) |
| gocritic | 25 | Low | Style: octal literals, importShadow, paramTypeCombine, etc. |
| contextcheck | 16 | Medium | Missing context propagation — goroutines losing request context |
| nilerr | 12 | Low-Med | Intentional patterns (file not found during walkDir) but could mask bugs |
| containedctx | 2 | Medium | context.Context embedded in struct (`memory.go:39`, `auth.go:346`) |
| gomoddirectives | 1 | Low | Local replace in go.mod (valid for development) |

## 4. golangci-lint Config Changes

Enhanced `.golangci.yml` with additional security-focused linters:
- Added: `bidichk`, `containedctx`, `contextcheck`, `depguard`, `errcheck`, `gocheckcompilerdirectives`, `gocritic`, `gomoddirectives`, `nilerr`, `nolintlint`, `nosprintfhostport`, `predeclared`, `spancheck`
- Added govet passes: `fieldalignment`, `shadow`, `nilness`, `lostcancel`, `sigchanyzer`, `sortslice`, `unusedwrite`
- Added depguard rules to block deprecated `log` and `io/ioutil` packages

## 5. Created Artifacts

| File | Content |
|------|---------|
| `/tmp/opencode/gosec-report.json` | Full gosec JSON output |
| `/tmp/opencode/govulncheck-report.json` | Full govulncheck JSON output |
| `/tmp/opencode/golangci-lint-report.json` | Full golangci-lint JSON output |

## 6. Next Action Items for Phase 1

1. Fix unhandled errors in `module/manager.go` (SetState, SetHealth failing silently)
2. Fix integer overflow conversions in gRPC server files (int→int32 casts)
3. Replace `math/rand` with `crypto/rand` in retry and load balancer
4. Audit goroutines using `context.Background()` — propagate request context
5. Add context propagation to `containedctx` violations
6. Review `nilerr` findings for hidden bugs
7. Address fieldalignment warnings for memory/cache optimization
