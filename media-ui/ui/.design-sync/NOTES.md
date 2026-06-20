# design-sync notes — media-ui

- **vite-plugin-dts v5 generates empty `export {}`** — `rollupTypes: true/false` both produce `export {}` in `dist/index.es.d.ts`. Workaround: `dist/index.d.ts` is hand-authored as the package entry point, with `"types": "dist/index.d.ts"` in `package.json`. Update this file whenever `src/lib.ts` changes.

- **Router context for page previews** — pages that use `useNavigate`/`useParams` (Layout, MediaCard, MovieDetail, Player, TVShowDetail) need a MemoryRouter wrapper. Solution: export `MemoryRouter as RouterWrapper` from `src/lib.ts`, then set `cfg.provider.component = "RouterWrapper"`. The converter wraps all preview renders in `h(window.MediaUI.RouterWrapper, {}, h(fn))`.

- **react-router-dom is inlined** — the converter inlines `@remix-run/router`, `react-router`, and `react-router-dom` into the bundle (listed under `inlinedExternals`). This is correct because they're needed at runtime in the preview. The RouterWrapper approach works because all router code shares the same inlined instance.

- **CSS_RUNTIME is expected** — Tailwind v4 via `@tailwindcss/vite` injects styles at runtime. No static CSS file is shipped. The bundle is self-styling.

- **MovieDetail / TVShowDetail / Spinner show thin previews** — these pages make API calls that fail in the preview environment (no backend). They correctly show a loading spinner. This is expected behavior, not a bug.

- **FONT_MISSING: Inter** — the CSS references "Inter" but no `@font-face` ships with the bundle. System fonts are substituted in previews. This is acceptable.

- **Build command for re-sync**: `npx vite build --config vite.config.lib.ts` (run from `ui/`), then run the converter. Also remember to rebuild the app build separately with `npm run build`.
