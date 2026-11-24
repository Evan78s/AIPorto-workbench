-- Aiporto Workbench 运营后台 / 客户控制台所需表。
-- 本文件不包含任何演示数据；租户、套餐、价格、key 全部由使用者自行填写。

CREATE TABLE IF NOT EXISTS plans (
    id                  TEXT PRIMARY KEY,
    code                TEXT NOT NULL UNIQUE,
    name                TEXT NOT NULL,
    monthly_token_limit BIGINT NOT NULL DEFAULT 1000000,
    rpm_limit           INT NOT NULL DEFAULT 60,
    concurrency_limit   INT NOT NULL DEFAULT 4,
    daily_token_limit   BIGINT NOT NULL DEFAULT 100000,
    price_per_month     NUMERIC(20,8) NOT NULL DEFAULT 0,
    currency            CHAR(3) NOT NULL DEFAULT 'CNY',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS subscriptions (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL UNIQUE REFERENCES tenants(id),
    plan_id         TEXT REFERENCES plans(id),
    quota_override  JSONB NOT NULL DEFAULT '{}'::jsonb,
    status          TEXT NOT NULL DEFAULT 'active',
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS billing_ledger (
    id            BIGSERIAL PRIMARY KEY,
    tenant_id     TEXT NOT NULL REFERENCES tenants(id),
    entry_type    TEXT NOT NULL CHECK (entry_type IN ('recharge','refund','adjustment')),
    amount        NUMERIC(20,8) NOT NULL CHECK (amount > 0),
    currency      CHAR(3) NOT NULL DEFAULT 'CNY',
    reference_id  TEXT NOT NULL UNIQUE,
    description   TEXT NOT NULL DEFAULT '',
    created_by    TEXT NOT NULL DEFAULT 'admin',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_ledger_tenant_time ON billing_ledger(tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS model_pricing (
    id                   TEXT PRIMARY KEY,
    model                TEXT NOT NULL UNIQUE,
    input_price_per_1m   NUMERIC(20,8) NOT NULL DEFAULT 0,
    output_price_per_1m  NUMERIC(20,8) NOT NULL DEFAULT 0,
    currency             CHAR(3) NOT NULL DEFAULT 'CNY',
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS console_tokens (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL REFERENCES tenants(id),
    token_hash    TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'active',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_console_tokens_tenant ON console_tokens(tenant_id, status);

-- 统一费用口径：价格表为空时费用为 0，不会报错。
CREATE OR REPLACE VIEW v_usage_cost AS
SELECT u.*,
       (u.prompt_tokens::numeric / 1000000.0 * COALESCE(mp.input_price_per_1m, 0)
        + u.completion_tokens::numeric / 1000000.0 * COALESCE(mp.output_price_per_1m, 0)) AS cost
FROM usage_logs u
LEFT JOIN model_pricing mp ON mp.model = u.model_resolved;

-- 余额 = 充值/退款/调整 - 已计量用量费用。
CREATE OR REPLACE VIEW v_tenant_balance AS
SELECT t.id AS tenant_id,
       COALESCE((SELECT SUM(l.amount) FROM billing_ledger l
                 WHERE l.tenant_id = t.id AND l.entry_type IN ('recharge','refund','adjustment')), 0)
       - COALESCE((SELECT SUM(c.cost) FROM v_usage_cost c WHERE c.tenant_id = t.id), 0) AS balance,
       COALESCE((SELECT SUM(c.cost) FROM v_usage_cost c
                 WHERE c.tenant_id = t.id AND c.created_at >= date_trunc('month', now())), 0) AS month_cost,
       COALESCE((SELECT SUM(c.prompt_tokens + c.completion_tokens) FROM v_usage_cost c
                 WHERE c.tenant_id = t.id AND c.created_at >= date_trunc('month', now())), 0) AS month_tokens
FROM tenants t;
