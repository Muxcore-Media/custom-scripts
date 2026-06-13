# Agent Conduct

- Never mention yourself, your name, your creator, or your nature in code, comments, commit messages, PR descriptions, or any other output.
- Never add attribution footers or any self-referential content to commits or pull requests.
- Write as if the human is the sole author of all work produced.
- Be concise. Answer directly. Never add preamble, postamble, or commentary unless asked.

## Agent Priorities (in order)

These priorities govern all agent decisions. When priorities conflict, the higher one wins.

1. **Security trumps everything.** Never introduce vulnerabilities, leak secrets, weaken boundaries, or cut corners on auth/audit/TLS/input validation. If a change has security implications, flag it explicitly and prefer the safer path even if it's slower or more complex.

2. **Code efficiency and speed.** Write performant code — avoid unnecessary allocations, goroutine leaks, blocking calls in hot paths, and redundant work. Prefer simple, tight solutions over over-engineered ones. Profile before optimizing, but don't write deliberately slow code.

3. **Never leave something unfinished.** If a task has remaining work, TODOs, unresolved issues, or blockers that prevent completion, you **must** explicitly state what's left at the end of your response and request to continue working on it. Mark the exact locations of unfinished code in the source with `TODO`, `FIXME`, `HACK`, or `XXX` so the continuation can find and remove them. Do not silently move on from incomplete work.

## Communication Protocol

- Answer in as few words as possible. One word when sufficient. Never add preamble or summary.
- When instructions are ambiguous, ask a clarifying question. Do not guess.
- When blocked, state the exact blocker and what is needed to proceed.
- Never explain your code or summarize your actions unless the user asks.

## Definition of Done

A task is done only when ALL of the following are true:
- All requested changes are implemented.
- All TODOs, FIXMEs, HACKs, and XXXs are resolved — none left behind.
- `go vet`, `golangci-lint`, and `staticcheck` pass with zero warnings.
- Unit tests pass with `-race`.
- `gofmt` produces no diffs.
- No new dependencies added without explicit justification.
- The change matches existing code conventions (style, patterns, libraries, test approach).

## Code Change Protocol

- Read a file before editing it.
- Match existing patterns — naming, typing, error handling, imports, test style.
- Prefer existing libraries and utilities. Never add a new dependency unless existing ones cannot do the job.
- Never add comments to code unless explicitly asked.
- When interrupted before finishing a change, mark all incomplete spots with `TODO`, `FIXME`, `HACK`, or `XXX` in the code. When resuming and completing that work, remove those markers.
- After any change, run `gofmt` and the relevant package tests.

## Failure Recovery

- If a command fails, read the error output and fix the root cause before retrying.
- If a test fails, diagnose and fix. Do not proceed with failing tests.
- If a tool returns an unexpected result, verify your assumption before proceeding.
- If stuck after two attempts, state the blocker explicitly and ask for guidance.

---

# Project Reference

## Git Remotes

The local `core/` repository has two remotes:

| Remote | URL | Purpose |
|--------|-----|---------|
| `origin` | `https://github.com/Muxcore-Media/core.git` | Public release repo |
| `private` | `https://github.com/TheMinecraftGuyGuru/core-dev.git` | Private development repo |

**Workflow:**
1. All development happens on `private`.
2. `origin` only receives clean, squashed commits representing complete, shippable work.
3. Before pushing to `origin`, rebase/squash the development history into clean, professional commits.
4. The public repo's history must remain clean — no test triggers, no CI back-and-forth, no messy iteration.

## Pre-shipment Checklist

Before shipping to the public repo, ALL of the following must pass:

1. `go vet ./internal/... ./pkg/... ./cmd/... ./sdk/...` — zero warnings
2. `golangci-lint run --timeout 120s ./internal/... ./pkg/... ./cmd/... ./sdk/...` — zero violations
3. `staticcheck ./internal/... ./pkg/... ./cmd/... ./sdk/...` — zero violations
4. `go test -race -count=1 -timeout 120s ./internal/... ./pkg/...` — all pass
5. `(cd sdk/go/mock && go test -race -count=1 ./...)` — all pass
6. `(cd sdk/go/client && go mod tidy && go test -race -count=1 ./...)` — all pass
7. `go test -tags=integration -race -count=1 -timeout 120s ./internal/integration/...` — all pass
8. All 8 CI jobs green (Lint, Test, Build, Coverage, Go Mod Verify, Docker Build, Vulnerability Scan, SLSA Provenance)
9. `gofmt -l` — no output
10. Commit history is clean, conventional, no test/CI artifacts

Fix any failure before shipping. Do not push broken or incomplete work to the public repo.

## Directory Layout

```
/home/enderk/claude/
├── core/                    # MuxCore distributed fabric (Go project)
├── core.wiki/               # Public-facing GitHub Wiki mirror
├── notes/                   # Private working notes (not committed to public)
├── <module>*/               # Module directories (auto-discovered by muxidx)
├── scripts/muxidx/          # Vector search indexer
└── AGENTS.md                # This file
```

Modules are auto-discovered by muxidx — any directory at the workspace root containing
`go.mod`, `muxcore.json`, `Cargo.toml`, or `main.py` is indexed as a searchable repo.

### `core/` Key Subsystems

| Directory | Purpose |
|-----------|---------|
| `cmd/muxcored/` | Server bootstrap |
| `internal/api/` | HTTP API server (auth, rate limit, audit, security headers) |
| `internal/config/` | Config loading, env var overlay, validation |
| `internal/module/` | Module lifecycle manager |
| `internal/module/mgr/` | Sidecar process manager |
| `internal/registry/` | Module registry with capability index and dependency resolution |
| `internal/events/` | In-memory event bus |
| `internal/grpcmesh/` | gRPC server/client, cluster discovery, auth, health, storage proxy |
| `internal/storage/` | Storage orchestrator with routing policies and cache |
| `internal/audit/` | JSONL audit logger with SHA-256 hash chain |
| `internal/trace/` | Trace ID propagation |
| `internal/spool/` | Tag fetching with SSRF protection |
| `pkg/contracts/` | Core-module contract interfaces |
| `proto/` | Protobuf definitions and generated gRPC code |
| `sdk/go/mock/` | Mock implementations for testing |
| `sdk/go/client/` | Go client SDK |
| `scripts/` | CI watchdog and wiki push |
| `docs/` | Additional documentation |

## Working Rules

- **Module capabilities are the security boundary.** Core enforces that modules can only do what their declared capabilities allow.
- **TLS is required in production.** `MUXCORE_INSECURE_DISABLE_TLS` is for development only.
- **Audit is fire-and-forget.** Audit calls use goroutines and must never block core operations.
- **gRPC sidecar model.** Modules run as separate processes, registering via gRPC. The in-process `contracts.Register` path is deprecated.

## Build & Test

```bash
cd core
make build        # Build muxcored binary
make test         # Core tests with race detection
make lint         # golangci-lint or go vet
make coverage     # Coverage report
make proto        # Regenerate protobuf code
make ci           # Full CI pipeline
```

## Agent Protocol — Token Efficiency

**Before reading any file or using grep, use muxidx.** The correct order:

1. **Orient** — `muxidx_search("what you're looking for")` to find relevant chunks across code + wiki.
2. **Drill** — `muxidx_get_chunk(chunk_id)` to read just the relevant function or section.
3. **Trace** — `muxidx_graph_walk(chunk_id, relation="calls")` to follow import/call chains.
4. **Read** — `Read` tool only when editing or when exact byte layout matters.

**Never** load a whole package to understand one function.
**Never** grep without first trying `muxidx_search`.

### MCP Tools

| Tool | Description |
|------|-------------|
| `muxidx_search(query, repo, top_k, include_graph)` | Semantic search across code + docs. Filter by `repo=` with a repo name (core, wiki, auth-local) or a capability tag (auth, cache, database, metrics, tracing, etc.). Comma-separated: `repo=auth,cache`. |
| `muxidx_graph_walk(chunk_id, relation, max_depth)` | Walk knowledge graph from a chunk |
| `muxidx_get_chunk(chunk_id)` | Get full content of a specific chunk |
| `muxidx_stats()` | Index statistics (coverage, counts) |

### Adding New Repos to the Search Index

When you create a new module or repo at the workspace root, **just create the directory with a recognized marker file** — indexing is automatic. No config changes needed.

**Discovery rules** — a directory at `/home/enderk/claude/` is indexed if it contains any of:
- `go.mod` (Go module)
- `muxcore.json` (MuxCore module)
- `Cargo.toml` (Rust crate)
- `main.py` (Python project)

**Tags assignment** — search tags are derived from the `capabilities` array in `muxcore.json`. For example:
```json
{
  "name": "My Module",
  "capabilities": ["my.feature", "storage"]
}
```
This makes the module searchable via `repo=my.feature` or `repo=storage`.
If no `muxcore.json` exists, a single tag is derived from the directory name (hyphens → dots).

**Exclusions verified automatically** — hidden dirs (`.git`, `.venv`, etc.) and the `scripts/`, `notes/`, `docs/` dirs are never indexed.

**Important for `muxcore-module-starter`**: if you clone the starter template as a new module, remember to update its `muxcore.json` with the correct module name and capabilities, or delete the placeholder `muxcore.json` so the auto-derived tag takes effect. The starter's placeholder capability `your.capability` will be used if left unchanged, which is not useful for searching — always customize it.
