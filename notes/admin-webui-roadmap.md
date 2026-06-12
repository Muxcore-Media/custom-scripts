# Admin Web UI — Roadmap

**Updated:** 2026-06-10
**Module:** `github.com/Muxcore-Media/admin-ui`
**Stack:** Go + Templ + HTMX + Tailwind CSS (standalone CLI)

---

## Phase 0: Foundation (Week 1)

| # | Task | Deps | Est. | Done when |
|---|------|------|------|-----------|
| 0.1 | Create Go module, `go.mod`, directory structure | — | 30m | `go build` succeeds, layout matches plan |
| 0.2 | Write `main.go`: dial core via `sdk/go/client`, serve empty HTTP | — | 1h | Binary starts, `curl localhost:PORT` returns something |
| 0.3 | Download standalone Tailwind CLI, write `input.css`, `tailwind.config.js`, `Makefile` build target | — | 30m | `make css` produces `assets/dist/styles.css` |
| 0.4 | Download HTMX, place in `assets/htmx.min.js` | — | 5m | File exists, gitignored path clear |
| 0.5 | Wire `//go:embed` for static assets, serve `/static/` | 0.3, 0.4 | 30m | `curl /static/dist/styles.css` returns CSS from RAM |
| 0.6 | Create `Makefile` with: `build`, `css`, `run`, `clean` | all above | 30m | `make build` produces single binary with embedded assets |
| 0.7 | Scaffold `templ/layout.templ` with Tailwind sidebar nav shell | 0.5 | 1h | Browser shows dark layout with sidebar + main area |
| 0.8 | Set CSP header on HTTP server (`default-src 'self'; script-src 'self'; style-src 'self'`) | 0.2 | 15m | `curl -I /` shows `Content-Security-Policy` header |
| 0.9 | Write `templ/login.templ` | 0.7 | 30m | Login form renders with Tailwind styling |
| 0.10 | Write `session/store.go` with `Create`, `Get`, `Revoke` | — | 30m | Unit test passes for create/get/revoke cycle |
| 0.11 | Write `handler/auth.go`: login handler, session middleware, logout | 0.9, 0.10 | 2h | Login form → cookie set → protected route accessible |
| 0.12 | Discover `AuthProvider` on startup, show "no auth" page if missing | 0.11 | 30m | Missing auth provider shows config error page |
| 0.13 | Write `handler/handler.go`: `Handler` struct, `requireAuth`, helper types | 0.11 | 1h | Auth middleware wired, all routes protected |

**Phase 0 deliverable:** Authenticated web server running, serving Tailwind-styled layout with sidebar nav, login/logout working against a real AuthProvider module.

---

## Phase 1: Dashboard + Module List (Week 2)

| # | Task | Deps | Est. | Done when |
|---|------|------|------|-----------|
| 1.1 | Write `templ/components/badge.templ` (health/state colored dot) | 0.7 | 30m | `@badge.Health("ok")` renders green dot |
| 1.2 | Write `templ/components/nav.templ` (sidebar with active state) | 0.7 | 30m | Nav links render, active page highlighted |
| 1.3 | Write `templ/components/spinner.templ` | 0.7 | 15m | CSS spinner renders for HTMX loading states |
| 1.4 | Write `templ/dashboard.templ` with health grid + cluster summary | 0.7 | 1h | Wireframe of health cards + cluster info boxes |
| 1.5 | Write `handler/dashboard.go`: `GET /`, `GET /dashboard` | 1.4 | 1h | Page shows real data from core's gRPC |
| 1.6 | Wire HTMX polling: `hx-get="/dashboard/health" hx-trigger="every 5s"` | 1.5 | 30m | Health card updates without page reload |
| 1.7 | Write `templ/module_list.templ` with table + state badges + cap tags | 0.7 | 1.5h | Table renders with all module columns |
| 1.8 | Write `templ/module_detail.templ` with deps graph, metadata | 0.7 | 1h | Detail page shows full module info |
| 1.9 | Write `handler/modules.go`: `GET /modules`, `GET /modules/{id}` | 1.7, 1.8 | 1.5h | `FindByRole("*")` drives list, `Resolve(id)` drives detail |
| 1.10 | Add `hx-boost="true"` to layout body for SPA nav | 1.9 | 15m | Clicking module link updates main content, no full reload |

**Phase 1 deliverable:** Dashboard with live-updating health indicators and cluster status. Module list with detail pages. Full SPA navigation via HTMX boost.

---

## Phase 2: Cluster + Events (Week 3)

| # | Task | Deps | Est. | Done when |
|---|------|------|------|-----------|
| 2.1 | Write `templ/cluster.templ` with node cards + leader badge | 0.7 | 1h | Node cards show ID, addr, modules, health |
| 2.2 | Write `handler/cluster.go`: `GET /cluster` | 2.1 | 1h | `c.Discovery.Members()` drives the page |
| 2.3 | Implement SSE endpoint `GET /cluster/events` streaming `Watch()` | 2.2 | 2h | Browser receives live cluster events via EventSource |
| 2.4 | Wire HTMX `hx-trigger="sse:..."` for live cluster updates | 2.3 | 1h | Node joins/leaves appear without refresh |
| 2.5 | Write `templ/event_stream.templ` with filter bar + event table | 0.7 | 1h | Table shows event type, source, timestamp |
| 2.6 | Write `handler/events.go`: `GET /events` | 2.5 | 1h | `c.Events.Subscribe()` feeds the stream |
| 2.7 | Add subscription stats panel (count per event type, drop rates) | 2.6 | 1h | Stats via `SubscriptionStats()` or new `EventService.Stats()` RPC |
| 2.8 | Add per-subscriber latency/dropped metrics display | 2.7 | 1h | `SubscriberStats()` drives a metrics table |

**Phase 2 deliverable:** Live cluster map with SSE-powered updates. Event stream viewer with subscription metrics.

---

## Phase 3: Storage + Settings (Week 4)

| # | Task | Deps | Est. | Done when |
|---|------|------|------|-----------|
| 3.1 | Write `templ/storage.templ` with provider cards + cap tags | 0.7 | 1h | Cards show provider name, capabilities, health |
| 3.2 | Write `handler/storage.go`: `GET /storage` | 3.1 | 1h | `c.Storage.Capabilities()` drives display |
| 3.3 | Write `templ/components/table.templ` (reusable sortable table) | 0.7 | 1h | Any page can render `@table.Sortable(...)` |
| 3.4 | Write `templ/settings.templ` with dynamic form rendering | 0.7 | 2h | `SettingDef` types map to form controls (input, select, toggle, masked secret) |
| 3.5 | Write `handler/settings.go`: `GET /settings`, `POST /settings/{module}/{key}` | 3.4 | 2h | Discovers SettingsProvider modules, renders forms, POST calls update via mesh |
| 3.6 | Wire HTMX on settings forms: `hx-post`, `hx-target="#msg"` | 3.5 | 30m | Form submit shows success/error inline |
| 3.7 | Add secret field masking (show/hide toggle) | 3.4 | 30m | Secret fields mask by default, toggle to reveal |

**Phase 3 deliverable:** Storage provider viewer. Dynamic settings UI — discovers `SettingsProvider` modules, renders typed forms, submits changes via mesh calls.

---

## Phase 4: Audit + Config + Polish (Week 5)

| # | Task | Deps | Est. | Done when |
|---|------|------|------|-----------|
| 4.1 | Write `templ/audit.templ` with filter form + result table + pagination | 0.7 | 2h | Filters for actor, action, time range, trace ID |
| 4.2 | Write `handler/audit.go`: `GET /audit`, pagination | 4.1 | 2h | Queries `AuditLogger.Query()` via mesh or gRPC |
| 4.3 | Write `templ/components/pagination.templ` | 0.7 | 30m | Page numbers, prev/next, current page highlight |
| 4.4 | Write `templ/config.templ` with key-value display, secrets masked | 0.7 | 1h | Redacted config rendered as grouped sections |
| 4.5 | Write `handler/config.go`: `GET /config` | 4.4 | 1h | Config from new `AdminService.Config()` RPC or env fallback |
| 4.6 | Write `templ/components/modal.templ` for destructive actions | 0.7 | 1h | `hx-confirm` fallback + custom modal component |
| 4.7 | Write `templ/error.templ` — error page with message, status code, trace ID | 0.7 | 30m | Errors render consistently with nav |
| 4.8 | Add loading states: spinner shown during HTMX swaps | 1.3 | 30m | `hx-indicator` on all slow endpoints |
| 4.9 | Add `hx-target="main"` error handling with `hx-target-error` | 4.8 | 30m | Failed swaps show error in-page, not broken layout |
| 4.10 | Responsive layout: sidebar collapses on mobile | 0.7 | 1h | Mobile nav toggle works |
| 4.11 | Dark mode audit: verify all colors pass contrast ratio | 4.10 | 1h | No invisible text, WCAG AA for readability |
| 4.12 | CSP hardening audit: verify no `unsafe-inline` or `unsafe-eval` needed | 0.8 | 30m | All inline styles eliminated, HTMX CSP-safe |
| 4.13 | End-to-end integration test: deploy core + test auth module + admin-ui | all | 2h | Walk through all pages against a real core instance |
| 4.14 | Documentation pass: README with arch diagram, config, auth setup | all | 1h | README covers build, configure, run, auth |

**Phase 4 deliverable:** Audit log browser with full filtering and pagination. Config viewer. Production polish: loading states, error handling, responsive layout, dark mode, CSP audit.

---

## Phase 5: Production Hardening (Week 6)

| # | Task | Deps | Est. | Done when |
|---|------|------|------|-----------|
| 5.1 | Rate limiting on login endpoint (per-IP, exponential backoff) | 0.11 | 1h | Brute-force protection matches core's auth failure tracking |
| 5.2 | Session expiry: configurable TTL, cleanup goroutine for stale sessions | 0.10 | 1h | Sessions expire after N minutes, cleanup runs every 5min |
| 5.3 | CSRF token double-submit cookie for mutating routes beyond Origin check | 0.11 | 1h | POST/PUT/DELETE require matching cookie + header |
| 5.4 | Audit logging of admin actions (login, logout, settings change, module stop) | 4.2 | 1h | Admin actions appear in core's audit log |
| 5.5 | Graceful shutdown: drain HTTP, close gRPC client, revoke sessions | 0.2 | 1h | SIGTERM → drain → close → exit cleanly |
| 5.6 | Prometheus metrics for admin module (requests, latency, errors, active sessions) | — | 1h | `/metrics` endpoint with standard Go metrics |
| 5.7 | HTTPS support: cert/key via env vars, TLS listener | 0.2 | 30m | `ADMIN_UI_TLS_CERT` + `ADMIN_UI_TLS_KEY` enable TLS |
| 5.8 | Config via env vars: port, core gRPC address, session TTL, TLS | — | 1h | All configurable via env, documented in README |
| 5.9 | `golangci-lint` + `staticcheck` pass, `go vet` clean | all | 1h | Zero lint or vet warnings |
| 5.10 | CI pipeline: lint, build, test, CSS build, binary size check | all | 1h | GitHub Actions: all green |

**Phase 5 deliverable:** Production-ready module: rate limiting, session expiry, CSRF tokens, audit trail, graceful shutdown, Prometheus metrics, TLS, CI pipeline.

---

## Post-MVP

| # | Task | Why | Priority |
|---|------|-----|----------|
| P.1 | WebSocket-based event push (replace HTMX polling for cluster/events) | Lower latency, fewer requests | Medium |
| P.2 | Module stop/restart from UI | Operator convenience; needs `Manager.StopModule` RPC in core first | Low |
| P.3 | Module log viewer (tail logs from sidecar modules) | Debugging; needs core to expose log streams | Low |
| P.4 | mTLS client cert auth for browser | Zero-password auth for internal tools | Medium |
| P.5 | SSO/OIDC via AuthProvider | Enterprise deployments | Low |
| P.6 | i18n — translatable strings | Multi-language operators | Very low |

---

## Dependency graph (simplified)

```
Phase 0 ──────────────────────────────────────────────────────────────┐
  ├── 0.1-0.6 (scaffold, build, embed)                                │
  ├── 0.7 (layout.templ) ─────────────────────────────────────────────┤
  ├── 0.8 (CSP)                                                        │
  ├── 0.9 (login.templ) ─── 0.11 (auth handler) ─── 0.13 (handler) ──┤
  └── 0.10 (session store) ──┘                                        │
                                                                       │
Phase 1 ───────────────────────────────────────────────────────────────┤
  ├── 1.1 (badge component) ──┐                                       │
  ├── 1.2 (nav component) ────┤                                       │
  ├── 1.3 (spinner) ──────────┼── 1.4 (dashboard.templ) ── 1.5-1.6 ──┤
  │                            │                                       │
  ├── 1.7 (module_list.templ) ─── 1.9 (modules handler) ─────────────┤
  └── 1.8 (module_detail.templ) ──┘                                   │
                                                                       │
Phase 2 ───────────────────────────────────────────────────────────────┤
  └── 2.1 → 2.2 → 2.3 → 2.4 (cluster with SSE)                       │
  └── 2.5 → 2.6 → 2.7 → 2.8 (events with stats)                      │
                                                                       │
Phase 3 ───────────────────────────────────────────────────────────────┤
  ├── 3.1 → 3.2 (storage)                                              │
  ├── 3.3 (table component, reusable)                                  │
  └── 3.4 → 3.5 → 3.6 → 3.7 (settings with forms)                    │
                                                                       │
Phase 4 ───────────────────────────────────────────────────────────────┤
  ├── 4.1 → 4.2 → 4.3 (audit with pagination)                         │
  ├── 4.4 → 4.5 (config)                                               │
  ├── 4.6-4.9 (modal, errors, loading states)                          │
  ├── 4.10-4.11 (responsive + dark mode)                               │
  ├── 4.12 (CSP audit)                                                 │
  ├── 4.13 (integration test)                                          │
  └── 4.14 (docs)                                                      │
                                                                       │
Phase 5 ───────────────────────────────────────────────────────────────┘
  └── All production hardening tasks independent, order arbitrary
```

---

## Effort summary

| Phase | Tasks | Est. time | Deliverable |
|-------|-------|-----------|-------------|
| Phase 0: Foundation | 13 | ~9h | Auth + layout working |
| Phase 1: Dashboard + Modules | 10 | ~8h | Live dashboard, module browsing |
| Phase 2: Cluster + Events | 8 | ~9h | Live cluster, event stream |
| Phase 3: Storage + Settings | 7 | ~8h | Storage viewer, dynamic settings forms |
| Phase 4: Audit + Config + Polish | 14 | ~14h | Full audit browser, production polish |
| Phase 5: Hardening | 10 | ~10h | Production-ready module |
| **Total** | **62** | **~58h** | |

**Calendar estimate:** 6 weeks with one developer (assuming ~10h/week on this).
**Parallelizable:** Phases 1-3 can overlap if two developers work in parallel.
