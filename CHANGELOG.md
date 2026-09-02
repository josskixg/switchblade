# Changelog

All notable changes to Switchblade are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [1.0.0] — 2026-06-27

### Major Release — Multi-Tenant SaaS Platform

This is the first production-ready release of Switchblade as a multi-tenant SaaS platform.
All previous single-tenant functionality has been extended with tenant isolation, user
authentication, subscription tiers, and a modern React dashboard.

### Added — Multi-Tenant Architecture

- **Tenants table** — isolated data per organization (`internal/db/schema_v2.sql`)
- **Users table** — username/password authentication with role-based access (owner/admin/developer/viewer)
- **Tiers table** — subscription plans with configurable limits (free/pro/enterprise)
- **Tenant isolation** — all business tables now scoped to `tenant_id`
- **Versioned DB migrations** — `internal/db/migrate.go` with `schema_migrations` tracking

### Added — Authentication & Security

- **JWT authentication** — pure stdlib HS256 implementation (`internal/auth/jwt.go`)
- **Bcrypt password hashing** — replaced SHA-256 with bcrypt + legacy fallback (`internal/auth/password.go`)
- **Login/Register/Logout/Me/Refresh** endpoints (`internal/api/auth.go`)
- **Default secret enforcement** — fails startup if JWT_SECRET, API_KEY, or ENCRYPTION_KEY are unchanged
- **Error message sanitization** — 59 instances of `err.Error()` replaced with generic messages (15 files)
- **SSRF prevention** — provider BaseURL validated against internal IP ranges (`internal/api/ssrf.go`)
- **Auth-protected SSE** — `/api/events` requires JWT (query param for EventSource compat)
- **Auth-protected metrics** — `/metrics` requires JWT or API key

### Added — Subscription System

- **Tier CRUD API** — owner can create/edit/delete tiers (`internal/api/tiers.go`)
- **Per-tier limits** — max API keys, max models, rate limit, daily/monthly token quotas
- **API key scoping** — keys restricted to specific model patterns (`gpt-*`, `claude-*`, `*`)
- **Usage tracking** — token recording per tenant per key (`internal/api/usage.go`)
- **Quota enforcement** — background goroutine auto-blocks tenants exceeding limits (`internal/api/quota_monitor.go`)
- **Usage stats** — daily/monthly usage dashboard with remaining quota

### Added — React Dashboard

- **Vite + React + Tailwind + shadcn/ui** — modern SPA replacing legacy HTML dashboard (`web/`)
- **Login/Register pages** — dark cyberpunk design with form validation
- **Dashboard layout** — sidebar navigation, topbar, responsive design
- **Overview page** — real-time stats cards, activity table, SSE live events
- **Accounts page** — full CRUD with modal forms
- **Keys page** — API key management with copy-to-clipboard
- **Usage page** — token usage charts, cost estimates, recent records
- **Admin panel** — tenant management, suspend/activate, tier assignment, platform stats
- **Production build** — 636KB JS + 24KB CSS, served by Go binary

### Added — CLI Tool

- **switchblade-cli** — 15+ subcommands for terminal management (`cmd/cli/main.go`)
- `init` — initialize database, create tables, seed tiers
- `start` — start the server
- `status` — show server status, DB version, counts
- `tenants list/create/suspend/activate/delete`
- `users list/create/delete`
- `keys create/list/delete` — with model scoping
- `tiers list/update`
- `backup` — database backup
- `stats` — platform-wide statistics
- `help` — command reference

### Added — DevOps

- **Cross-platform CI/CD** — GitHub Actions builds for linux/windows/darwin on amd64/arm64/386/riscv64/loong64/mips64/freebsd
- **Docker support** — multi-stage build with distroless runtime (~15MB image)
- **Docker Compose** — healthcheck, volumes, environment variables
- **Release automation** — tagged releases with auto-generated changelog and binary artifacts
- **Version injection** — `-ldflags` injects version, commit hash, build date into binary
- **/version endpoint** — returns version, commit, date, Go version
- **OpenAPI 3.0.3 spec** — complete API documentation (`docs/openapi.yaml`)
- **k8s health checks** — `/health` (uptime), `/ready` (DB ping), `/live` (liveness)
- **Prometheus metrics** — wired to `/metrics` endpoint

### Changed

- **Dashboard auth** — API key auth now scoped to tenant, JWT auth added as alternative
- **Password storage** — new hashes use bcrypt; legacy SHA-256 hashes still verifiable
- **Error responses** — all 500-level errors return generic "internal server error" message
- **Binary size** — maintained at <20MB with new features

### Security

- 19 security findings identified and resolved: 5 CRITICAL, 5 HIGH, 4 MEDIUM, 3 LOW, 2 INFO
- No known exploitable vulnerabilities at time of release

---

## [Unreleased]

### In Progress
- `POST /v1/images/generations` proxy endpoint (route to Canva/Fireworks, download + store image locally)
- `GET /v1/images/retrieve/{id}` — serve stored images from disk with correct Content-Type
- Auto-cleanup via `IMAGE_RETENTION_DAYS` cron job
- Python auth scripts (`scripts/auth/login.py`, `canva_*.py`, per-provider bots)
- VCC auto-assign logic (assign card to account needing upgrade, track result)
- Dashboard SPA (`dashboard/`) — React + embedded via `//go:embed`
- `relay_nodes` table in `schema.sql` (currently created inline in `relay_mgmt.go`)

---

## [0.11.0] — 2026-06-26

### Added — Phase 11: Management API (Complete)

- `GET/POST/PUT/DELETE /api/accounts` — full account pool CRUD (`internal/api/management.go`)
- `GET /api/stats` + `GET /api/logs` — usage stats and request log viewer
- `GET/POST/PUT/DELETE /api/keys` — API key management with bcrypt hashing (`internal/api/keys.go`)
- `GET/POST/PUT/DELETE /api/filters` — PUDIDIL filter rule CRUD
- `GET /api/relay/nodes` — relay node listing (`internal/api/relay_mgmt.go`)
- `GET/POST/PUT/DELETE /api/model-combos` — fallback chain CRUD (`internal/api/combos.go`)
- `GET/POST/PUT/DELETE /api/proxy-pool` — egress proxy pool management (`internal/api/proxypool.go`)
- `GET/POST/PUT/DELETE /api/webhooks` — webhook endpoint management (`internal/api/webhooks.go`)
- `GET/POST /api/export`, `POST /api/import` — JSON export/import (`internal/api/export.go`)
- `GET /api/cache/stats`, `DELETE /api/cache/purge` — cache management (`internal/api/cache.go`)
- `GET/POST/DELETE /api/vcc/cards`, `GET /api/vcc/transactions` — VCC pool management (`internal/api/vcc.go`)
- `POST /api/replay` — failed request replay queue (`internal/api/replay.go`)
- `GET /api/images`, `GET /api/images/stats`, `DELETE /api/images/{id}` — image studio management (`internal/api/images.go`)
- `GET /v1/models` — list all available models (`internal/api/models.go`)

---

## [0.9.0] — 2026-06-26

### Added — Phase 9: Relay System (Complete)

- Relay protocol over HTTP with JSON encoding (`internal/relay/protocol.go`)
- Relay client mode — connects to relay server, forwards requests (`internal/relay/client.go`)
- Relay server mode — accepts relay clients, `X-Relay-Secret` auth (`internal/relay/server.go`)
- Cloudflared binary lifecycle manager — auto-download, health-check 30s, watchdog 10s, auto-reconnect (`internal/relay/tunnel/manager.go`)
- Relay management API (`internal/api/relay_mgmt.go`)

> **Note:** Protocol implemented as JSON over HTTP (stdlib only), not CBOR/WebSocket as originally planned. `gorilla/websocket` and `fxamacker/cbor` not yet added.

---

## [0.8.0] — 2026-06-26

### Added — Phase 8: Auth Automation (Complete)

- `LoginQueue` — Python subprocess job queue, concurrency 3, max 10 queued, exponential backoff (`internal/auth/queue.go`)
- `WarmupQueue` — API-only health check with semaphore concurrency control (`internal/auth/warmup.go`)
- `AutoWarmupScheduler` — recurring 15-minute timer persisted in `settings` table (`internal/auth/scheduler.go`)
- Python subprocess integration via `exec.Command` in `queue.go`

> **Note:** Python browser bot scripts (`scripts/auth/`) not yet written.

---

## [0.5.0] — 2026-06-26

### Added — Phase 5: Model Mapping + Filters + Combos (Complete)

- Model alias resolution with DB-cached 10s TTL (`internal/proxy/modelmap.go`)
- PUDIDIL content filter engine — hot-reloadable regex rules (`internal/proxy/filters.go`)
- Model combos CRUD endpoints + `model_combos` table (`internal/api/combos.go`)
- `custom_models` table in schema
- Streaming fallback engine — max 3 attempts, logs full `fallback_chain` per request (`internal/proxy/fallback.go`)

---

## [0.4.0] — 2026-06-26

### Added — Phase 4: Token Compression Pipeline (Complete)

- 6-stage compression orchestrator (`internal/proxy/compression/pipeline.go`):
  - **Stage 1 TSC** — JSON schema whitespace/description stripping
  - **Stage 2 DCP** — cross-turn content deduplication
  - **Stage 3 RTK** — tool result truncation in old turns
  - **Stage 4 Caveman** — git diff, directory tree, repetitive content compression
  - **Stage 5 ImageDedupe** — base64 image block deduplication across turns
  - **Stage 6 CacheMarkers** — Anthropic `cache_control` insertion at stable prefixes
- Decompression utility for replay/debug (`internal/proxy/decompress.go`)
- Compression stats written to `request_logs.compression_stats` per request

> **Note:** All 6 stages implemented in `pipeline.go` rather than split into separate files as originally planned.

---

## [0.3.0] — 2026-06-26

### Added — Phase 3: Operational Features (Complete)

- SQLite WAL auto-backup with gzip compression (`internal/db/backup.go`)
- Pre-startup auto-backup to `data/backups/pre-startup-{timestamp}.db`
- `switchblade restore --file <path>` CLI command
- Backup rotation — keeps N most recent backups (default: 10)
- Request log pruning — auto-delete after N days (`LOG_RETENTION_DAYS`)
- Usage summary pruning — auto-delete after N days (`USAGE_RETENTION_DAYS`)
- Request replay queue (`/api/replay`) with `replay_status` state machine
- JSON data export (`/api/export`) and import (`/api/import`)
- Response cache layer — prompt hash → SQLite (`internal/proxy/cache.go`)
- Cache bypass header `X-Cache-Bypass: true`
- Cache stats endpoint (`/api/cache/stats`)
- Token-bucket rate limiting per API key (`internal/proxy/ratelimit.go`)
- Webhook delivery system with retry + exponential backoff (`internal/api/webhooks.go`)
- `webhook_logs` table for delivery tracking
- Prometheus-compatible `/metrics` endpoint (`internal/metrics/metrics.go`)

---

## [0.2c.0] — 2026-06-26

### Added — Phase 2c: Auth-Bot Provider Stubs (Complete)

Provider Go stubs for browser-auth providers (Python bots not yet written):
- `canva.go` — Canva AI (browser cookies)
- `kiropro.go` — Kiro Pro (OAuth, higher quota)
- `qoder.go` — Qoder (Bearer token, custom protocol)
- `codebuddy.go`, `codex.go`, `mimo.go` — additional auth-bot providers

---

## [0.2b.0] — 2026-06-26

### Added — Phase 2b: API-Key Providers (18/18 Complete)

All API-key-based providers implemented:

- `openrouter.go` — 100+ models, free-tier available
- `together.go` — Llama, Mistral, Qwen ($25 trial)
- `groq.go` — Llama-3, Mixtral, Gemma (30 req/min free, ultra-fast)
- `fireworks.go` — Llama, Mixtral, Stable Diffusion ($10 credit)
- `perplexity.go` — Llama-3.1 Sonar models
- `mistral.go` — Large, Small, Codestral
- `deepseek.go` — V3, Coder (generous free quota)
- `cohere.go` — Command R, Embed, Rerank (100 calls/min)
- `ai21.go` — Jamba, Jurassic-2 (trial credits)
- `novita.go` — Llama, Mistral (cheap API)
- `hyperbolic.go` — Llama, SDXL (fast inference)
- `cerebras.go` — Llama-3.1 @ ultra-fast
- `sambanova.go` — Llama-3.1-70B, 405B (trial)
- `githubmodels.go` — OpenAI/Llama via GitHub Marketplace (free PAT)
- `googleai.go` — Gemini 1.5 Flash/Pro (generous free tier)
- `cloudflareai.go` — Llama models (10k/day free)
- `huggingface.go` — 1000+ models (20k/day free)
- `alibaba.go` — Qwen via DashScope (free tier)

---

## [0.2.0] — 2026-06-26

### Added — Phase 2: Core Proxy (Complete)

- `Provider` interface + `BaseProvider` implementation (`internal/providers/base.go`)
- Provider registry with priority order (`internal/providers/registry.go`)
- `AccountPool` — active account cache with configurable TTL (`internal/proxy/pool.go`)
- Load balancing strategies: `round_robin`, `least_conn`, `random`, `weighted`
- Request routing pipeline with 13-stage orchestration (`internal/proxy/router.go`)
- BYOK provider — any OpenAI-compatible endpoint with user's own key (`internal/providers/byok.go`)
- Kiro provider — AWS CodeWhisperer OAuth, fallback position (`internal/providers/kiro.go`)
- OpenAI provider (`internal/providers/openai.go`)
- Anthropic provider (`internal/providers/anthropic.go`)

---

## [0.1.0] — 2026-06-26

### Added — Phase 1: Foundation (Complete)

- `go mod init switchblade` — pure Go project, no CGO
- Dependencies: `chi/v5` (router), `modernc.org/sqlite` (pure Go SQLite)
- Config struct with all env var loading + defaults (`internal/config/config.go`)
- SQLite connection + idempotent 14-table schema migration (`internal/db/db.go`, `schema.sql`)
- Basic HTTP server with chi router (`cmd/switchblade/main.go`)
- API key authentication middleware (`internal/api/middleware.go`)
- CORS middleware
- `GET /health` — status, uptime, active accounts, DB size
- `GET /ready` — readiness probe (DB connection + active accounts > 0)
- `Makefile` with `build`, `run`, `test`, `clean`, `migrate` targets
- `.env.example` with all 38 configurable env vars

---

[Unreleased]: https://github.com/yourorg/switchblade/compare/v0.11.0...HEAD
[0.11.0]: https://github.com/yourorg/switchblade/compare/v0.9.0...v0.11.0
[0.9.0]: https://github.com/yourorg/switchblade/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/yourorg/switchblade/compare/v0.5.0...v0.8.0
[0.5.0]: https://github.com/yourorg/switchblade/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/yourorg/switchblade/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/yourorg/switchblade/compare/v0.2c.0...v0.3.0
[0.2c.0]: https://github.com/yourorg/switchblade/compare/v0.2b.0...v0.2c.0
[0.2b.0]: https://github.com/yourorg/switchblade/compare/v0.2.0...v0.2b.0
[0.2.0]: https://github.com/yourorg/switchblade/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/yourorg/switchblade/releases/tag/v0.1.0
