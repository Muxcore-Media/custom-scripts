# Admin Web UI — Module Plan

**Updated:** 2026-06-10
**Stack:** Go + Templ + HTMX + Tailwind CSS (standalone CLI)
**Status:** Roadmap created — see `admin-webui-roadmap.md`
**Target:** 6 weeks, ~58h effort
**Decision:** Admin UI removed from core (PR #71). Core only provides data.

---

## Why it's a module, not core

Core's rule: if a feature could be a module, it should be a module. The admin
UI is a perfect example — some operators want a web dashboard, some want CLI
only, some want to build their own. Hardcoding an HTML UI into core violates
the loom philosophy.

---

## Architecture

Sidecar module running as a separate Go binary. It dials core's gRPC endpoint
using the existing `sdk/go/client`, serves its own HTTP server with Templ +
HTMX, and embeds all static assets (Tailwind CSS, HTMX JS) via `//go:embed`
for zero-disk I/O at runtime.

```
┌─────────────────────────────────────┐
│  muxcored                           │
│  ┌───────────────┐  ┌────────────┐ │
│  │ DiscoverySvc  │  │ EventSvc   │ │
│  │ HealthSvc     │  │ StorageSvc │ │
│  │ ModuleMesh    │  │            │ │
│  └───────┬───────┘  └─────┬──────┘ │
└──────────┼─────────────────┼────────┘
           │ gRPC            │ gRPC
┌──────────┼─────────────────┼────────┐
│  admin-ui (separate binary)         │
│  ┌──────┴──────┐  ┌───────┴──────┐ │
│  │ Go SDK      │  │              │ │
│  │ Client      │  │  HTTP Server │ │
│  └──────┬──────┘  │  ┌────────┐  │ │
│         │ data    │  │ Templ  │  │ │
│         │         │  │ HTMX   │  │ │
│         │         │  │ Tailwind│  │ │
│         │         │  └────────┘  │ │
│         │         └──────┬───────┘ │
│         │         │ browser HTTP   │
└─────────┼─────────┼────────────────┘
          │         │
      gRPC (core)   HTTPS (browser ←→ admin-ui)
```

## What core provides (the seam for this module)

Core exposes data accessors that the admin module consumes via the gRPC SDK.
These stay in core:

| Data | Core Accessor | How admin-ui gets it |
|------|---------------|----------------------|
| Registered modules | `reg.ListAll()` | `c.Discovery.FindByRole("*")` |
| Cluster membership | `discoveryGrpc.MembersSnapshot()` | `c.Discovery.Members()` |
| Leader / term | `discoveryGrpc.IsLeader()`, `.Term()` | `c.Discovery.Members()` response includes leader_id |
| Cluster events (live) | `discoveryGrpc.Watch()` | `c.Discovery.Watch()` streaming |
| Event subscriptions | `bus.SubscriptionStats()` | New `c.Events.Stats()` gRPC RPC |
| Storage capabilities | `store.ProviderInfo()` | `c.Storage.Capabilities()` |
| Running config | `cfg.RedactedJSON()` | New `c.Admin.Config()` gRPC RPC or env |
| Module settings | `contracts.SettingsProvider` | `c.Mesh.Call(target, "Settings", nil)` |
| Build version | `version.String()` | `c.Health.Check("")` or `Discovery.Resolve("")` |
| Prometheus metrics | `/metrics` endpoint | Direct HTTP scrape (stays in core) |
| pprof | `/debug/pprof/*` | Direct HTTP (stays in core, gated) |

## Planned module: `admin-ui`

**Repo:** `github.com/Muxcore-Media/admin-ui`
**Role:** `admin`
**Capabilities:** `admin.web`

### What it provides

- **Dashboard** — system overview: module health grid, cluster status, event throughput
- **Module list** — all registered modules with state, health, capabilities, dependency graph
- **Cluster map** — nodes, leader, term, module distribution per node (live via Watch stream)
- **Event stream** — live event bus viewer with subscription counts, drop rates, latency
- **Storage status** — storage provider capabilities and health
- **Settings UI** — discovers `SettingsProvider` modules, renders typed forms (string, int, bool, select, secret)
- **Audit log browser** — query audit entries with filters (actor, action, time range, trace ID)
- **Config viewer** — running config (redacted secrets, safe for viewing)

### Tailwind CSS asset pipeline

Use the **standalone Tailwind CLI binary** — no Node.js, no npm, no Vite.

**Build step** (scans `.templ` and `.go` files for class names, outputs minified CSS):

```bash
./tailwindcss -i ./input.css -o ./assets/dist/styles.css --minify
```

**input.css** (minimal, just directives):

```css
@tailwind base;
@tailwind components;
@tailwind utilities;
```

**Embed into Go binary** — CSS served from RAM, zero disk I/O:

```go
package main

import (
    "embed"
    "net/http"
)

//go:embed assets/dist/*
//go:embed assets/htmx.min.js
var staticAssets embed.FS

func main() {
    mux := http.NewServeMux()
    mux.Handle("/static/", http.FileServer(http.FS(staticAssets)))
    // ...
}
```

**Cost:** A single ~15KB gzipped CSS file, served from memory, containing only
the utility classes actually used in your `.templ` files.

### Repo layout

```
admin-ui/
├── main.go                    # Binary entrypoint — dials core, starts HTTP
├── go.mod
├── go.sum
├── Makefile
├── input.css                  # Tailwind directives
├── tailwind.config.js
├── assets/
│   ├── dist/
│   │   └── styles.css         # Built by tailwindcss CLI (gitignored)
│   └── htmx.min.js            # Downloaded once, embedded forever
├── handler/
│   ├── handler.go             # Handler struct, common helpers
│   ├── dashboard.go           # GET /, GET /dashboard
│   ├── modules.go             # GET /modules, GET /modules/{id}
│   ├── cluster.go             # GET /cluster
│   ├── events.go              # GET /events
│   ├── storage.go             # GET /storage
│   ├── settings.go            # GET /settings, POST /settings/{module}/{key}
│   ├── audit.go               # GET /audit
│   ├── config.go              # GET /config
│   └── auth.go                # GET /login, POST /login, GET /logout, session middleware
├── templ/
│   ├── layout.templ           # Base layout with nav sidebar, Tailwind classes
│   ├── dashboard.templ        # Health grid, cluster summary widgets
│   ├── module_list.templ      # Full table w/ state badges, capability tags
│   ├── module_detail.templ    # Single module: metadata, deps, settings form
│   ├── cluster.templ          # Node cards, leader indicator, module distribution
│   ├── event_stream.templ     # Filterable event table, subscription stats panel
│   ├── storage.templ          # Provider list with capability tags
│   ├── settings.templ         # Dynamic forms rendered from SettingDef
│   ├── audit.templ            # Filter form + results table
│   ├── config.templ           # Key-value config display
│   ├── login.templ            # Login form
│   ├── error.templ            # Error page (generic)
│   └── components/
│       ├── badge.templ        # Health/state badge (green/yellow/red dot)
│       ├── table.templ        # Reusable table component
│       ├── modal.templ        # Confirmation modal for mutating actions
│       ├── nav.templ          # Sidebar navigation
│       ├── pagination.templ   # Pagination controls
│       └── spinner.templ      # Loading indicator for HTMX swaps
├── session/
│   └── store.go               # Server-side session store (sync.Map, cookie-backed)
└── client.go                  # Core gRPC client wrapper (or uses SDK directly)
```

### Templ + Tailwind layout example

```templ
// templ/layout.templ
package templ

templ Layout(title string, nav templ.Component, content templ.Component) {
    <!DOCTYPE html>
    <html lang="en" class="h-full bg-gray-950">
    <head>
        <meta charset="UTF-8"/>
        <meta name="viewport" content="width=device-width, initial-scale=1"/>
        <title>{ title } - MuxCore Admin</title>
        <link rel="stylesheet" href="/static/dist/styles.css"/>
        <script src="/static/htmx.min.js"></script>
    </head>
    <body class="h-full" hx-boost="true">
        <div class="flex h-full">
            <nav class="w-64 shrink-0 border-r border-gray-800">
                @nav
            </nav>
            <main class="flex-1 overflow-y-auto p-6">
                @content
            </main>
        </div>
    </body>
    </html>
}
```

### HTMX patterns

- **Live health badges**: `hx-get="/dashboard/health" hx-trigger="every 5s" hx-target="#health-grid"` — server returns a `<div>` with fresh health indicators
- **Smooth navigation**: `hx-boost="true"` on `<body>` — all internal links become AJAX, history works, no full page reload
- **Event stream**: `hx-get="/events/stream" hx-trigger="every 2s"` or the admin module opens an SSE/WebSocket for true push
- **Settings form submission**: `hx-post="/settings/{module}/{key}" hx-target="#msg-{key}" hx-swap="innerHTML"` — instant feedback
- **Modal confirmations**: `hx-delete="/modules/{id}" hx-confirm="Stop module?" hx-target="#module-list"` — built-in HTMX confirm dialog

### Auth model

- Stateless session tokens stored in a server-side `sync.Map` keyed by random UUID cookie
- On login, admin module calls `AuthProvider.Authenticate()` via `MeshClient.Call()`
- On each request, reads session cookie, calls `Authorizer.Can(session, "admin.access", "admin.ui")` via `MeshClient.Call()`
- If no `AuthProvider` is discovered (`FindByCapability("auth")` returns empty), every route returns a "no auth provider configured" page (safe default)
- CSRF: `Origin` header check on all POST/PUT/DELETE routes (same approach as core's `securityHeadersMiddleware`). HTMX sends the `Origin` header automatically on same-origin requests.

### Session store

```go
// session/store.go
type Store struct {
    mu     sync.RWMutex
    tokens map[string]*contracts.Session // token → session
}

func (s *Store) Create(session contracts.Session) (token string, err error)
func (s *Store) Get(token string) (*contracts.Session, bool)
func (s *Store) Revoke(token string)
```

### Handler pattern

```go
// handler/handler.go
type Handler struct {
    core     *client.Client             // gRPC SDK client
    sessions *session.Store
    mux      *http.ServeMux
}

func (h *Handler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        cookie, err := r.Cookie("session")
        if err != nil {
            http.Redirect(w, r, "/login", http.StatusSeeOther)
            return
        }
        sess, ok := h.sessions.Get(cookie.Value)
        if !ok {
            http.Redirect(w, r, "/login", http.StatusSeeOther)
            return
        }
        // Verify permission via Authorizer
        allowed, err := h.checkPermission(r.Context(), *sess, "admin.access", "admin.ui")
        if err != nil || !allowed {
            http.Error(w, "Forbidden", http.StatusForbidden)
            return
        }
        ctx := context.WithValue(r.Context(), "session", sess)
        next(w, r.WithContext(ctx))
    }
}
```

### Implementation order

1. **Scaffold** — `go mod init`, `main.go` dials core via SDK, serves a "hello" Templ page. Standalone Tailwind CLI in `Makefile`. `//go:embed` for CSS + HTMX. CSP header set.
2. **Auth** — `session.Store`, login page, `POST /login` calls `AuthProvider.Authenticate()` via mesh, cookie set. Logout. `requireAuth` middleware. "No auth provider" fallback page.
3. **Layout + Dashboard** — `layout.templ` with Tailwind sidebar nav, `dashboard.templ` with health grid (HTMX polling every 5s), cluster summary widgets.
4. **Module list & detail** — table of all modules (`FindByRole("*")`), click for detail page with deps, health, caps.
5. **Cluster map** — node cards from `Members()`, leader badge, live updates via `Watch()` stream pushed to an SSE endpoint.
6. **Events** — subscribe to event bus via `EventsClient.Subscribe()`, display subscription stats, drop rates, per-subscriber latency.
7. **Settings** — discover `SettingsProvider` modules, render typed forms per `SettingDef`, handle POST to update (via mesh call).
8. **Audit log** — filter form (actor, action, time range, trace ID), results table with pagination, export button.
9. **Config** — redacted config display (key-value pairs, secrets masked).
10. **Polish** — loading spinners for HTMX swaps, error states, responsive layout, dark mode refinement, CSP hardening.

### What's NOT needed in core

- HTML rendering (admin module's job — Templ compiles to Go)
- CSRF middleware (admin module checks Origin on mutating routes)
- `/admin/*` routes (admin module serves its own HTTP server)
- Admin-specific auth logic (uses standard `Authorizer` + `AuthProvider` via mesh)

### Potential new gRPC RPCs in core

The admin module works with existing gRPC services, but two additions would avoid
kludges:

1. **`EventService.Stats()`** — returns subscription counts, drop rates, publish
   count. Currently `MemoryBus.SubscriptionStats()` is in-memory only; a sidecar
   can't reach it without a gRPC endpoint.

2. **`AdminService.Config()`** — returns core's redacted running config as JSON.
   Currently `cfg.RedactedJSON()` exists in `internal/config/` but has no gRPC
   exposure. The admin module could read env vars as a fallback, but a proper
   RPC is cleaner.

### SDK / CLI alternative

Operators who don't want a web UI can use `muxcorectl` (separate repo,
to be planned) which talks directly to core's gRPC services:

```
muxcorectl modules list
muxcorectl cluster status
muxcorectl events tail
muxcorectl storage ls
muxcorectl audit query --actor user42 --limit 50
```

---

## Contracts already in core

| Contract | File | Notes |
|----------|------|-------|
| `SettingsProvider` | `pkg/contracts/settings.go` | Modules expose their config fields; admin UI renders them |
| `AuditLogger` (Query/Export) | `pkg/contracts/audit.go` | Admin module queries audit entries |
| `Authorizer` | `pkg/contracts/auth.go` | Admin module enforces `admin.access` permission |
| `IdentityProvider` | `pkg/contracts/identity.go` | Admin module extracts session identity |
| `AuthProvider` | `pkg/contracts/auth.go` | Login: Authenticate + Validate + Revoke |

No new contracts needed for a basic admin UI module. All necessary hooks exist.

---

## Speed architecture summary

| Layer | Technique | Effect |
|-------|-----------|--------|
| HTML | Templ compiles to raw Go functions | Zero runtime reflection, bytes written directly to `io.Writer` |
| CSS | Standalone Tailwind CLI, embedded via `embed.FS` | ~15KB minified, served from RAM, no Node.js pipeline |
| JS | HTMX single ~14KB gzipped file, embedded | No bundle step, no npm, no hydration |
| Data | gRPC streaming via SDK client | Binary protocol, no JSON parse overhead on server |
| Network | Admin module runs its own HTTP server | Core's middleware chain doesn't process admin HTML routes |
| CSP | `default-src 'self'; script-src 'self'; style-src 'self'` | Strict policy, no `unsafe-eval`, no inline scripts |
