-- Switchblade schema v2 — multi-tenant edition
-- All timestamps are integer (unix seconds) unless noted otherwise.
-- Every business table has tenant_id for tenant isolation.

-- ═══════════════════════════════════════════════════════════════
-- tenants — tenant / business entity
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS tenants (
    id          TEXT PRIMARY KEY,
    name        TEXT    NOT NULL,
    email       TEXT    NOT NULL,
    status      TEXT    NOT NULL DEFAULT 'active',
    created_at  INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    updated_at  INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    tier_id     TEXT    REFERENCES tiers(id)
);
CREATE UNIQUE INDEX IF NOT EXISTS tenants_email_idx ON tenants (email);

-- ═══════════════════════════════════════════════════════════════
-- tiers — subscription plans
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS tiers (
    id                   TEXT PRIMARY KEY,
    name                 TEXT    NOT NULL UNIQUE,
    description          TEXT,
    max_api_keys         INTEGER NOT NULL DEFAULT 5,
    max_models           INTEGER NOT NULL DEFAULT 10,
    rate_limit_per_minute INTEGER NOT NULL DEFAULT 60,
    daily_token_limit    INTEGER NOT NULL DEFAULT 100000,
    monthly_price_cents  INTEGER NOT NULL DEFAULT 0,
    created_at           INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);

-- ═══════════════════════════════════════════════════════════════
-- users — auth within a tenant
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS users (
    id             TEXT PRIMARY KEY,
    tenant_id      TEXT    NOT NULL REFERENCES tenants(id),
    username       TEXT    NOT NULL UNIQUE,
    password_hash  TEXT    NOT NULL,
    role           TEXT    NOT NULL DEFAULT 'developer',  -- owner|admin|developer|viewer
    email          TEXT,
    status         TEXT    NOT NULL DEFAULT 'active',
    created_at     INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    last_login     INTEGER
);
CREATE UNIQUE INDEX IF NOT EXISTS users_tenant_username_idx ON users (tenant_id, username);

-- ═══════════════════════════════════════════════════════════════
-- usage_records — per-request token accounting per tenant
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS usage_records (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id        TEXT    NOT NULL REFERENCES tenants(id),
    api_key_id       INTEGER REFERENCES api_keys(id),
    model            TEXT    NOT NULL,
    prompt_tokens    INTEGER DEFAULT 0,
    completion_tokens INTEGER DEFAULT 0,
    total_tokens     INTEGER DEFAULT 0,
    created_at       INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    cost_cents       INTEGER DEFAULT 0
);
CREATE INDEX IF NOT EXISTS usage_records_tenant_created_idx ON usage_records (tenant_id, created_at);
CREATE INDEX IF NOT EXISTS usage_records_model_idx          ON usage_records (model, created_at);

-- ═══════════════════════════════════════════════════════════════
-- api_key_scopes — per-key model access patterns
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS api_key_scopes (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    api_key_id  INTEGER NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    model_pattern TEXT  NOT NULL DEFAULT '*',            -- e.g. 'gpt-*', '*', 'claude-*'
    created_at  INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);
CREATE INDEX IF NOT EXISTS api_key_scopes_api_key_idx ON api_key_scopes (api_key_id);

-- ═══════════════════════════════════════════════════════════════
-- accounts — core entity, one row per AI service account
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS accounts (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id       TEXT    NOT NULL DEFAULT '_system' REFERENCES tenants(id),
    provider        TEXT    NOT NULL,               -- kiro|kiro-pro|codebuddy|canva|codex|qoder|byok|mimo
    email           TEXT    NOT NULL,
    password        TEXT    NOT NULL,               -- AES-256-GCM encrypted
    status          TEXT    NOT NULL DEFAULT 'pending',  -- pending|active|exhausted|error
    enabled         INTEGER NOT NULL DEFAULT 1,
    tokens          TEXT,                           -- JSON blob (access_token, refresh_token, …)
    quota_limit     REAL    DEFAULT 0,
    quota_remaining REAL    DEFAULT 0,
    quota_reset_at  INTEGER,
    last_used_at    INTEGER,
    last_login_at   INTEGER,
    error_message   TEXT,
    metadata        TEXT,                           -- JSON — extra provider-specific data
    tier            TEXT    NOT NULL DEFAULT 'free', -- subscription|cheap|free
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER
);
CREATE UNIQUE INDEX IF NOT EXISTS accounts_tenant_provider_email_idx ON accounts (tenant_id, provider, email);

-- ═══════════════════════════════════════════════════════════════
-- tier_config — account tier definitions with priority ordering
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS tier_config (
    tier       TEXT    PRIMARY KEY,
    priority   INTEGER NOT NULL,                    -- 1=highest, 3=lowest
    max_cost   REAL    NOT NULL DEFAULT 0,          -- max cost per request (0=unlimited)
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);

-- Seed tier_config defaults
INSERT OR IGNORE INTO tier_config (tier, priority, max_cost, enabled) VALUES ('subscription', 1, 0, 1);
INSERT OR IGNORE INTO tier_config (tier, priority, max_cost, enabled) VALUES ('cheap', 2, 0, 1);
INSERT OR IGNORE INTO tier_config (tier, priority, max_cost, enabled) VALUES ('free', 3, 0, 1);

-- ═══════════════════════════════════════════════════════════════
-- request_logs — audit trail per request
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS request_logs (
    id                     INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id              TEXT    NOT NULL DEFAULT '_system' REFERENCES tenants(id),
    account_id             INTEGER REFERENCES accounts(id),
    provider               TEXT    NOT NULL,
    model                  TEXT,
    service_kind           TEXT    NOT NULL DEFAULT 'chat',  -- chat|embeddings|tts|stt|image
    prompt_tokens          INTEGER DEFAULT 0,
    completion_tokens      INTEGER DEFAULT 0,
    total_tokens           INTEGER DEFAULT 0,
    credits_used           REAL    DEFAULT 0,
    status                 TEXT    NOT NULL,         -- success|error
    duration_ms            INTEGER,
    error_message          TEXT,
    request_body           TEXT,                    -- JSON
    response_body          TEXT,                    -- JSON
    account_email          TEXT,
    account_quota_before   REAL    DEFAULT 0,
    account_quota_after    REAL    DEFAULT 0,
    compression_stats      TEXT,                    -- JSON CompressionStats
    fallback_chain         TEXT,                    -- JSON array of model fallback attempts
    replay_status          TEXT,                    -- pending|success|error
    created_at             INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS request_logs_tenant_created_idx          ON request_logs (tenant_id, created_at);
CREATE INDEX IF NOT EXISTS request_logs_created_at_idx              ON request_logs (created_at);
CREATE INDEX IF NOT EXISTS request_logs_status_created_at_idx       ON request_logs (status, created_at);
CREATE INDEX IF NOT EXISTS request_logs_provider_created_at_idx     ON request_logs (provider, created_at);
CREATE INDEX IF NOT EXISTS request_logs_provider_model_status_idx   ON request_logs (provider, model, status);
CREATE INDEX IF NOT EXISTS request_logs_account_idx                 ON request_logs (account_id);

-- ═══════════════════════════════════════════════════════════════
-- usage_summary — hourly rollup for dashboard charts
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS usage_summary (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id         TEXT    NOT NULL DEFAULT '_system' REFERENCES tenants(id),
    bucket            TEXT    NOT NULL,             -- ISO-8601 hour bucket
    provider          TEXT    NOT NULL,
    model             TEXT    NOT NULL,
    request_count     INTEGER DEFAULT 0,
    prompt_tokens     INTEGER DEFAULT 0,
    completion_tokens INTEGER DEFAULT 0,
    total_tokens      INTEGER DEFAULT 0,
    credits_used      REAL    DEFAULT 0,
    total_duration_ms INTEGER DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS usage_summary_tenant_bucket_provider_model_idx ON usage_summary (tenant_id, bucket, provider, model);
CREATE INDEX IF NOT EXISTS usage_summary_tenant_bucket_idx     ON usage_summary (tenant_id, bucket);
CREATE INDEX IF NOT EXISTS usage_summary_bucket_idx            ON usage_summary (bucket);
CREATE INDEX IF NOT EXISTS usage_summary_provider_idx          ON usage_summary (provider, bucket);

-- ═══════════════════════════════════════════════════════════════
-- settings — key/value runtime config
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS settings (
    key        TEXT    NOT NULL,
    tenant_id  TEXT    NOT NULL DEFAULT '_system' REFERENCES tenants(id),
    value      TEXT,
    updated_at INTEGER,
    PRIMARY KEY (key, tenant_id)
);

-- ═══════════════════════════════════════════════════════════════
-- filter_rules — PUDIDIL content filter rules
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS filter_rules (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id    TEXT    NOT NULL DEFAULT '_system' REFERENCES tenants(id),
    rule_id      TEXT    NOT NULL,  -- UNIQUE per tenant
    pattern      TEXT    NOT NULL,
    replacement  TEXT    NOT NULL DEFAULT '',
    is_active    INTEGER NOT NULL DEFAULT 1,
    is_regex     INTEGER NOT NULL DEFAULT 0,
    sort_order   INTEGER NOT NULL DEFAULT 0,
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER
);
CREATE UNIQUE INDEX IF NOT EXISTS filter_rules_tenant_rule_id_idx ON filter_rules (tenant_id, rule_id);
CREATE INDEX IF NOT EXISTS filter_rules_sort_order_idx            ON filter_rules (sort_order);

-- ═══════════════════════════════════════════════════════════════
-- model_mappings — model alias rewrite (DB-backed, cached)
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS model_mappings (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    source_pattern TEXT    NOT NULL,                -- e.g. "haiku", "claude-3-5-sonnet"
    match_type     TEXT    NOT NULL DEFAULT 'contains', -- contains|exact|regex
    target_model   TEXT    NOT NULL DEFAULT '',     -- model id in the pool
    enabled        INTEGER NOT NULL DEFAULT 1,
    priority       INTEGER NOT NULL DEFAULT 0,     -- lower = evaluated first
    label          TEXT,                           -- human-readable label
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER
);
CREATE INDEX IF NOT EXISTS model_mappings_priority_idx ON model_mappings (priority);

-- ═══════════════════════════════════════════════════════════════
-- model_combos — named fallback chains
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS model_combos (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id   TEXT    NOT NULL DEFAULT '_system' REFERENCES tenants(id),
    name        TEXT    NOT NULL,                 -- combo name used as model id
    label       TEXT,                              -- human-readable label
    models_json TEXT    NOT NULL DEFAULT '[]',     -- JSON array of model ids
    enabled     INTEGER NOT NULL DEFAULT 1,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER
);
CREATE UNIQUE INDEX IF NOT EXISTS model_combos_tenant_name_idx ON model_combos (tenant_id, name);
CREATE INDEX IF NOT EXISTS model_combos_name_idx ON model_combos (name);

-- ═══════════════════════════════════════════════════════════════
-- custom_models — user-defined model → provider mapping
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS custom_models (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    model_id       TEXT    NOT NULL UNIQUE,
    owned_by       TEXT    NOT NULL,               -- e.g. "antigravity", "kiro", "byok"
    context_window INTEGER DEFAULT 200000,
    max_output     INTEGER DEFAULT 65536,
    thinking       INTEGER NOT NULL DEFAULT 0,
    vision         INTEGER NOT NULL DEFAULT 0,
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER
);
CREATE INDEX IF NOT EXISTS custom_models_model_id_idx ON custom_models (model_id);

-- ═══════════════════════════════════════════════════════════════
-- proxy_pool — egress HTTP/SOCKS5 proxies
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS proxy_pool (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id       TEXT    NOT NULL DEFAULT '_system' REFERENCES tenants(id),
    url             TEXT    NOT NULL,
    type            TEXT    NOT NULL DEFAULT 'http', -- http|socks5
    label           TEXT,
    status          TEXT    NOT NULL DEFAULT 'active', -- active|disabled|error
    last_used_at    INTEGER,
    last_checked_at INTEGER,
    error_message   TEXT,
    latency_ms      INTEGER,
    success_count   INTEGER DEFAULT 0,
    fail_count      INTEGER DEFAULT 0,
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER
);
CREATE INDEX IF NOT EXISTS proxy_pool_tenant_status_idx ON proxy_pool (tenant_id, status);
CREATE INDEX IF NOT EXISTS proxy_pool_status_idx        ON proxy_pool (status);

-- ═══════════════════════════════════════════════════════════════
-- vcc_cards — virtual credit card pool
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS vcc_cards (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id           TEXT    NOT NULL DEFAULT '_system' REFERENCES tenants(id),
    number              TEXT    NOT NULL,
    bin                 TEXT,
    exp_month           TEXT    NOT NULL,
    exp_year            TEXT    NOT NULL,
    cvv                 TEXT    NOT NULL,
    name                TEXT    DEFAULT 'John Doe',
    status              TEXT    NOT NULL DEFAULT 'active', -- active|used|declined
    used_by_account_id  INTEGER REFERENCES accounts(id),
    success_count       INTEGER DEFAULT 0,
    fail_count          INTEGER DEFAULT 0,
    created_at          INTEGER NOT NULL,
    updated_at          INTEGER
);
CREATE INDEX IF NOT EXISTS vcc_cards_tenant_status_idx ON vcc_cards (tenant_id, status);
CREATE INDEX IF NOT EXISTS vcc_cards_status_idx        ON vcc_cards (status);

-- ═══════════════════════════════════════════════════════════════
-- vcc_transactions — card charge history
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS vcc_transactions (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id      INTEGER REFERENCES accounts(id),
    card_id         INTEGER REFERENCES vcc_cards(id),
    card_last4      TEXT,
    card_bin        TEXT,
    card_brand      TEXT,
    amount          REAL,
    currency        TEXT    DEFAULT 'usd',
    status          TEXT    NOT NULL,               -- success|declined|error
    stripe_charge_id TEXT,
    created_at      INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS vcc_transactions_account_idx ON vcc_transactions (account_id);
CREATE INDEX IF NOT EXISTS vcc_transactions_status_idx  ON vcc_transactions (status);

-- ═══════════════════════════════════════════════════════════════
-- image_studio_chats — chat history for image generation
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS image_studio_chats (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    title       TEXT,
    messages    TEXT    NOT NULL DEFAULT '[]',     -- JSON array
    final_prompt TEXT,
    options     TEXT    DEFAULT '[]',              -- JSON array
    assist_model TEXT,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER
);
CREATE INDEX IF NOT EXISTS image_studio_chats_updated_at_idx ON image_studio_chats (updated_at);

-- ═══════════════════════════════════════════════════════════════
-- image_studio_results — generated images/videos (images)
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS image_studio_results (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id     TEXT    NOT NULL DEFAULT '_system' REFERENCES tenants(id),
    chat_id       INTEGER REFERENCES image_studio_chats(id) ON DELETE SET NULL,
    prompt        TEXT    NOT NULL,
    type          TEXT    NOT NULL DEFAULT 'image',
    aspect_ratio  TEXT    NOT NULL DEFAULT '1:1',
    n             INTEGER NOT NULL DEFAULT 1,
    urls          TEXT    NOT NULL DEFAULT '[]',   -- JSON array
    credits_used  REAL    DEFAULT 0,
    local_path    TEXT,                           -- local file path if downloaded
    file_size     INTEGER,                       -- file size in bytes
    file_type     TEXT,                          -- MIME type (image/png, video/mp4, …)
    created_at    INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS image_studio_results_tenant_created_idx ON image_studio_results (tenant_id, created_at);
CREATE INDEX IF NOT EXISTS image_studio_results_created_at_idx     ON image_studio_results (created_at);
CREATE INDEX IF NOT EXISTS image_studio_results_chat_idx           ON image_studio_results (chat_id);

-- ═══════════════════════════════════════════════════════════════
-- response_cache — cached LLM responses by prompt hash
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS response_cache (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    model         TEXT    NOT NULL,
    prompt_hash   TEXT    NOT NULL,                -- SHA-256 of canonicalised prompt
    response_body TEXT    NOT NULL,                -- full response JSON
    expires_at    INTEGER NOT NULL,               -- unix timestamp
    hit_count     INTEGER DEFAULT 0,
    created_at    INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS response_cache_model_prompt_hash_idx ON response_cache (model, prompt_hash);
CREATE INDEX IF NOT EXISTS response_cache_expires_at_idx               ON response_cache (expires_at);

-- ═══════════════════════════════════════════════════════════════
-- provider_config — per-provider base URL + model overrides
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS provider_config (
    provider   TEXT    NOT NULL,
    tenant_id  TEXT    NOT NULL DEFAULT '_system' REFERENCES tenants(id),
    base_url   TEXT    NOT NULL DEFAULT '',
    models     TEXT    NOT NULL DEFAULT '',  -- comma-separated list of model prefixes/names
    enabled    INTEGER NOT NULL DEFAULT 1,
    extra      TEXT    NOT NULL DEFAULT '{}', -- JSON for future knobs
    updated_at INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (tenant_id, provider)
);
CREATE INDEX IF NOT EXISTS provider_config_tenant_idx ON provider_config (tenant_id);

-- ═══════════════════════════════════════════════════════════════
-- api_keys — API key management
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS api_keys (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id    TEXT    NOT NULL DEFAULT '_system' REFERENCES tenants(id),
    name         TEXT    NOT NULL,
    key_hash     TEXT    NOT NULL,
    created_at   INTEGER NOT NULL,
    last_used_at INTEGER NOT NULL DEFAULT 0,
    enabled      INTEGER NOT NULL DEFAULT 1
);
CREATE UNIQUE INDEX IF NOT EXISTS api_keys_tenant_key_hash_idx ON api_keys (tenant_id, key_hash);

-- ═══════════════════════════════════════════════════════════════
-- webhooks — webhook endpoint definitions
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS webhooks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id   TEXT    NOT NULL DEFAULT '_system' REFERENCES tenants(id),
    url         TEXT    NOT NULL,
    events      TEXT    NOT NULL DEFAULT '[]',     -- JSON array of event names
    enabled     INTEGER NOT NULL DEFAULT 1,
    secret      TEXT,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER
);
CREATE INDEX IF NOT EXISTS webhooks_tenant_enabled_idx ON webhooks (tenant_id, enabled);

-- ═══════════════════════════════════════════════════════════════
-- webhook_logs — event delivery tracking
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS webhook_logs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id   TEXT    NOT NULL DEFAULT '_system' REFERENCES tenants(id),
    event       TEXT    NOT NULL,                  -- request_log|account_status|etc
    url         TEXT    NOT NULL,                  -- target URL
    payload     TEXT,                             -- JSON payload sent
    status      TEXT    NOT NULL DEFAULT 'pending', -- pending|success|error
    status_code INTEGER,
    error_message TEXT,
    retries     INTEGER DEFAULT 0,
    created_at  INTEGER NOT NULL,
    delivered_at INTEGER
);
CREATE INDEX IF NOT EXISTS webhook_logs_tenant_event_idx     ON webhook_logs (tenant_id, event);
CREATE INDEX IF NOT EXISTS webhook_logs_event_idx            ON webhook_logs (event);
CREATE INDEX IF NOT EXISTS webhook_logs_status_idx           ON webhook_logs (status);
CREATE INDEX IF NOT EXISTS webhook_logs_created_at_idx       ON webhook_logs (created_at);

-- ═══════════════════════════════════════════════════════════════
-- relay_nodes — relay/edge node definitions
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS relay_nodes (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id   TEXT    NOT NULL DEFAULT '_system' REFERENCES tenants(id),
    name        TEXT    NOT NULL,
    url         TEXT    NOT NULL,
    api_key     TEXT    NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 1,
    latency_ms  INTEGER DEFAULT 0,
    status      TEXT    NOT NULL DEFAULT 'active',
    created_at  INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    updated_at  INTEGER
);
CREATE INDEX IF NOT EXISTS relay_nodes_tenant_status_idx ON relay_nodes (tenant_id, status);

-- ═══════════════════════════════════════════════════════════════
-- SEED DATA — default tiers
-- ═══════════════════════════════════════════════════════════════

-- Free tier
INSERT OR IGNORE INTO tiers (id, name, description, max_api_keys, max_models, rate_limit_per_minute, daily_token_limit, monthly_price_cents)
VALUES ('tier_free', 'free', 'Free plan for evaluation', 5, 10, 60, 100000, 0);

-- Pro tier
INSERT OR IGNORE INTO tiers (id, name, description, max_api_keys, max_models, rate_limit_per_minute, daily_token_limit, monthly_price_cents)
VALUES ('tier_pro', 'pro', 'Professional plan', 50, 100, 600, 1000000, 2900);

-- Enterprise tier
INSERT OR IGNORE INTO tiers (id, name, description, max_api_keys, max_models, rate_limit_per_minute, daily_token_limit, monthly_price_cents)
VALUES ('tier_enterprise', 'enterprise', 'Enterprise plan with unlimited keys and models', -1, -1, 6000, -1, 9900);

-- Default tenant (for single-tenant backward compatibility)
INSERT OR IGNORE INTO tenants (id, name, email, tier_id)
VALUES ('tenant_default', 'Default Organization', 'admin@switchblade.local', 'tier_free');

-- ═══════════════════════════════════════════════════════════════
-- mitm_sessions — intercepted IDE request log
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS mitm_sessions (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    ide          TEXT    NOT NULL,
    provider     TEXT    NOT NULL,
    account_id   INTEGER,
    method       TEXT    NOT NULL,
    path         TEXT    NOT NULL,
    status_code  INTEGER NOT NULL DEFAULT 0,
    bytes_sent   INTEGER NOT NULL DEFAULT 0,
    created_at   INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);

-- ═══════════════════════════════════════════════════════════════
-- mitm_certs — cached per-domain TLS certificates
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS mitm_certs (
    domain       TEXT PRIMARY KEY,
    cert_pem     BLOB    NOT NULL,
    key_pem      BLOB    NOT NULL,
    expires_at   INTEGER NOT NULL,
    created_at   INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);

-- ═══════════════════════════════════════════════════════════════
-- seed — default tenant (must run after all tables exist: the FK target
-- check on tenants.tier_id requires every referenced table to exist)
-- ═══════════════════════════════════════════════════════════════
-- Many tables default to tenant_id='_system'; the row must exist or those
-- inserts fail once foreign keys are enforced (PRAGMA foreign_keys=1 in DSN).
INSERT INTO tenants (id, name, email, status)
VALUES ('_system', 'System', 'system@localhost', 'active')
ON CONFLICT(id) DO NOTHING;
