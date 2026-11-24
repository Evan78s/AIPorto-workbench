-- Aiporto Workbench MVP schema.
-- MVP 阶段 id 使用 TEXT（便于异步批量写入）；生产版可平滑切到 UUID + pgtype.UUID。

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS tenants (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'active',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS api_keys (
    id                 TEXT PRIMARY KEY,
    tenant_id          TEXT NOT NULL REFERENCES tenants(id),
    name               TEXT NOT NULL DEFAULT '',
    key_prefix         TEXT NOT NULL,
    key_hash           TEXT NOT NULL UNIQUE,
    status             TEXT NOT NULL DEFAULT 'active',
    expires_at         TIMESTAMPTZ,
    rpm_limit          INT NOT NULL DEFAULT 60,
    concurrency_limit  INT NOT NULL DEFAULT 4,
    daily_token_limit  BIGINT NOT NULL DEFAULT 1000000,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_api_keys_tenant_status ON api_keys(tenant_id, status);

CREATE TABLE IF NOT EXISTS upstream_providers (
    id        TEXT PRIMARY KEY,
    code      TEXT NOT NULL UNIQUE,
    base_url  TEXT NOT NULL,
    protocol  TEXT NOT NULL DEFAULT 'openai_compatible',
    status    TEXT NOT NULL DEFAULT 'active'
);

CREATE TABLE IF NOT EXISTS upstream_keys (
    id             TEXT PRIMARY KEY,
    provider_id    TEXT NOT NULL REFERENCES upstream_providers(id),
    tenant_id      TEXT REFERENCES tenants(id),
    name           TEXT NOT NULL DEFAULT '',
    encrypted_key  BYTEA NOT NULL,
    key_hint       TEXT NOT NULL DEFAULT '',
    priority       INT NOT NULL DEFAULT 100,
    weight         INT NOT NULL DEFAULT 1 CHECK (weight > 0),
    status         TEXT NOT NULL DEFAULT 'active'
);
CREATE INDEX IF NOT EXISTS idx_upstream_keys_provider ON upstream_keys(provider_id, status, priority);

CREATE TABLE IF NOT EXISTS routing_rules (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT REFERENCES tenants(id),
    model_alias  TEXT NOT NULL,
    provider_id  TEXT NOT NULL REFERENCES upstream_providers(id),
    model        TEXT NOT NULL,
    priority     INT NOT NULL DEFAULT 100,
    enabled      BOOLEAN NOT NULL DEFAULT true
);
CREATE INDEX IF NOT EXISTS idx_routing_alias ON routing_rules(model_alias, enabled, priority);

CREATE TABLE IF NOT EXISTS usage_logs (
    id                BIGSERIAL PRIMARY KEY,
    request_id        TEXT NOT NULL UNIQUE,
    tenant_id         TEXT NOT NULL,
    api_key_id        TEXT,
    provider_id       TEXT,
    upstream_key_id   TEXT,
    model_alias       TEXT NOT NULL,
    model_resolved    TEXT NOT NULL,
    endpoint          TEXT NOT NULL,
    stream            BOOLEAN NOT NULL DEFAULT false,
    prompt_tokens     INT NOT NULL DEFAULT 0,
    completion_tokens INT NOT NULL DEFAULT 0,
    status_code       INT NOT NULL,
    latency_ms        INT NOT NULL DEFAULT 0,
    ttft_ms           INT NOT NULL DEFAULT 0,
    cost_amount       NUMERIC(20,8) NOT NULL DEFAULT 0,
    currency          CHAR(3) NOT NULL DEFAULT 'CNY',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_usage_tenant_time ON usage_logs(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_usage_key_time ON usage_logs(api_key_id, created_at DESC);
