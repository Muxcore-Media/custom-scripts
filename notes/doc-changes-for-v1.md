# Documentation Changes for v1 (deferred)

This file tracks documentation changes that need to be made to the public wiki
(`core.wiki/`) when the v1 PR is pushed from the dev repo to the public repo.

## Remaining Items

### 1. Writing-Modules.md — Fix sidecar registration example

**File:** `core.wiki/Writing-Modules.md`
**Lines:** ~176-182

The sidecar entry point example uses the deprecated `info_json` field:

```go
resp, err := regClient.Register(context.Background(), &modulev1.RegisterRequest{
    ModuleId: *moduleID,
    InfoJson: infoJSON,
    MeshAddr: *meshAddr,
})
```

Needs to be replaced with the structured `ModuleInfo` field:

```go
info := mod.Info()
resp, err := regClient.Register(context.Background(), &modulev1.RegisterRequest{
    ModuleId:   *moduleID,
    ModuleInfo: &modulev1.ModuleInfo{
        Id:             info.ID,
        Name:           info.Name,
        Version:        info.Version,
        Roles:          info.Roles,
        Description:    info.Description,
        Author:         info.Author,
        Capabilities:   info.Capabilities,
        DependsOn:      info.DependsOn,
        MinCoreVersion: info.MinCoreVersion,
        HttpAddr:       info.HTTPAddr,
    },
    MeshAddr: *meshAddr,
})
```

Also need to add a section on MinCoreVersion and how version compatibility works.

### 2. Home.md — Verify accuracy

Ensure the `muxcored --tag default` example still works and the Quick Start
section references the correct registration flow.

### 3. Getting-Started.md

Verify that any mention of the in-process module path or Fabric is updated.

### 4. SDK Go README.md (core/sdk/go/README.md)

The test example in README.md at line 160 previously used `contracts.Fabric` for
dependency injection. This was changed to use individual mock parameters. Ensure
the final version is consistent.

## Post-v1 Doc Changes (June 2026)

- [ ] Update wiki pages to reference CHANGELOG.md for release history
- [ ] Document RBAC enforcement status (now "Implemented" per SECURITY.md update)
- [ ] Document GoReleaser release workflow for maintainers
- [ ] Document pre-commit hook auto-wiring via `make hooks`
- [ ] Update CI documentation to reference pinned action versions and vulncheck enforcement

---

## Completed

- ✅ Contracts.md — RouteRegistrar section removed
- ✅ Configuration-Reference.md — MUXCORE_STRICT_* env vars removed
- ✅ Event-System.md — MUXCORE_STRICT_PUBLISH_POLICY reference removed
- ✅ Architecture.md — Fabric struct section removed (only a brief reference remains)

### 5. New gRPC Services — Document in Wiki

Three new gRPC services were added to core for runtime management by admin tools:

**New proto files:**
- `proto/muxcore/spool/v1/spool.proto` — `SpoolService` with `ListSpools`, `ListTags`, `FetchTag`, `DeployTag`
- `proto/muxcore/lifecycle/v1/lifecycle.proto` — `ModuleLifecycleService` with `ListModules`, `SpawnModule`, `StopModule`, `RestartModule`
- `proto/muxcore/audit/v1/audit.proto` — `AuditService` with `Query`, `Export`, `VerifyChain`

**Changes to existing protos:**
- `proto/muxcore/discovery/v1/discovery.proto` — Added `state` and `health_error` fields to `ModuleInfoProto`, added `ListAll` RPC returning `ModuleEntryProto` (info + state + health + node_id)

**Wiki pages to update:**
- `Architecture.md` — Add the 3 new services to "The Five gRPC Services" section (now 8 services), add them to the bootstrap sequence
- `Spool-and-Marketplace.md` — Add `SpoolService` section showing runtime tag deployment via gRPC

**New wiki page needed?**
- `Admin-API.md` — reference for admin tools: list all gRPC management services with example calls

### Architecture.md — Bootstrap sequence step 12 (stale)
**File:** `core.wiki/Architecture.md`
**Line:** 12 in the numbered list

> 12. Wire admin handler → /admin/* endpoints registered on HTTP server

The admin handler was removed from core (PR #71). Delete this step. Also renumber steps 13-25 → 12-24.

#### Bootstrap order differs slightly from code
The decomposed `main()` initializes storage before the HTTP server, while the wiki lists HTTP server (step 13) before storage (step 14). These are functionally equivalent — the ordering doesn't affect behavior — but the wiki should be updated to match the actual bootstrap order in `cmd/muxcored/main.go`.

### Core-Concepts.md — "Fabric at a Glance" diagram (stale)
**File:** `core.wiki/Core-Concepts.md`
**Lines:** ~267-298

The diagram references "Admin UI (HTMX + Go templates)" and "API Gateway (REST + OpenAPI)" as boxes within the core process. These are neither in core nor planned for core — admin-ui is a separate module, and there is no separate API gateway. The diagram should show the 5 gRPC services, the event bus, registry, storage orchestrator, and the sidecar module manager (matching Architecture.md's component diagram).
