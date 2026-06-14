# Deleted Public Repos from muxcore-media (June 12, 2026)

These repos contained stale/empty module stubs based on an older core revision.
All deleted during v1 cleanup. New modules being built from scratch using the
updated `muxcore-module-starter` template.

## Deleted Contracts (16)
- contracts-artwork
- contracts-content
- contracts-discovery
- contracts-downloader
- contracts-filewatcher
- contracts-importlist
- contracts-search
- contracts-media
- contracts-mediainfo
- contracts-metadata
- contracts-notification
- contracts-playback
- contracts-quality
- contracts-resolver
- contracts-tag
- contracts-transcoder
- contracts-workflow

## Deleted Modules (25)
- admin-ui
- api-rest
- audit-logger
- auth-local
- auth-oidc
- cache-memory
- cache-redis
- database-postgres
- downloader module (republished elsewhere)
- eventbus-nats
- health-monitor
- searcher module (republished elsewhere)
- jellyfin
- media-library
- media-manager-movies
- muxcore-helm-chart
- muxcore-operator
- notifier-discord
- prometheus-metrics
- ratelimit-tokenbucket
- scheduler-cron
- storage-s3
- storage-tiering
- worker-pool
- workflow-engine

## Kept
- `core` — main repository
- `spool` — official module spool
- `contracts-reconciler` — Go dependency used by core
- `muxcore-module-starter` — module template
- `claude-working-directory` — private, dev workspace
