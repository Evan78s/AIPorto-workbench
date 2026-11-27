-- Aiporto Workbench B 方案：用户自助注册 / 登录 / 购买。
-- 设计红线：
--   1. 验证码、密码、会话令牌只存哈希，明文只通过邮件/一次响应出现。
--   2. 订单号、支付流水号唯一约束保证回调幂等，重复回调不会重复开通。
--   3. 用户与租户一对一：users.id -> tenants.owner_user_id，租户仍走原有鉴权/限流。
--   4. 不写任何演示数据。

CREATE TABLE IF NOT EXISTS users (
    id                TEXT PRIMARY KEY,
    email             TEXT NOT NULL UNIQUE,
    password_hash     TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'active',
    email_verified_at TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);

-- 邮箱验证码：同一邮箱 + 用途只保留一条有效记录，新的验证码作废旧验证码。
CREATE TABLE IF NOT EXISTS email_tokens (
    id          TEXT PRIMARY KEY,
    email       TEXT NOT NULL,
    purpose     TEXT NOT NULL CHECK (purpose IN ('register', 'reset_password')),
    code_hash   TEXT NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    attempts    INT NOT NULL DEFAULT 0,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_email_tokens_lookup ON email_tokens(email, purpose, created_at DESC);

-- 用户会话：token_hash 为 HMAC-SHA256 摘要，明文只返回给浏览器一次。
CREATE TABLE IF NOT EXISTS user_sessions (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_user_sessions_user ON user_sessions(user_id, expires_at DESC);

-- 租户反向关联到用户；一个用户一个租户。
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS owner_user_id TEXT REFERENCES users(id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_tenants_owner_unique ON tenants(owner_user_id) WHERE owner_user_id IS NOT NULL;

-- 订单：状态机 pending -> paid / cancelled / failed；paid 后可 refunded。
CREATE TABLE IF NOT EXISTS orders (
    id               TEXT PRIMARY KEY,
    order_no         TEXT NOT NULL UNIQUE,
    user_id          TEXT NOT NULL REFERENCES users(id),
    tenant_id        TEXT REFERENCES tenants(id),
    plan_id          TEXT NOT NULL REFERENCES plans(id),
    amount           NUMERIC(20,8) NOT NULL CHECK (amount >= 0),
    currency         CHAR(3) NOT NULL DEFAULT 'CNY',
    status           TEXT NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending','paid','cancelled','failed','refunded')),
    payment_channel  TEXT NOT NULL DEFAULT 'manual'
                     CHECK (payment_channel IN ('alipay','wechat','manual')),
    external_id      TEXT NOT NULL DEFAULT '',
    paid_at          TIMESTAMPTZ,
    granted_key      BOOLEAN NOT NULL DEFAULT false,
    -- 明文密钥仅在支付成功后短暂暂存（AES-GCM 加密），用户领取一次后清空。
    pending_credentials BYTEA,
    credentials_taken_at TIMESTAMPTZ,
    pay_payload      TEXT NOT NULL DEFAULT '', -- 下单响应的 JSON（二维码链接/对公文案），便于刷新后重看
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_orders_user ON orders(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status, created_at DESC);

-- 支付流水：外部流水号 + 渠道唯一，保证回调幂等。原始回调留痕便于对账。
CREATE TABLE IF NOT EXISTS payments (
    id           TEXT PRIMARY KEY,
    order_id     TEXT NOT NULL REFERENCES orders(id),
    channel      TEXT NOT NULL CHECK (channel IN ('alipay','wechat','manual')),
    external_id  TEXT NOT NULL,
    amount       NUMERIC(20,8) NOT NULL CHECK (amount >= 0),
    status       TEXT NOT NULL DEFAULT 'pending'
                 CHECK (status IN ('pending','succeeded','failed','refunded')),
    raw_notify   JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (channel, external_id)
);
CREATE INDEX IF NOT EXISTS idx_payments_order ON payments(order_id, created_at DESC);

-- 退款流水：退款必须关联到一条已成功的支付。
CREATE TABLE IF NOT EXISTS refunds (
    id           TEXT PRIMARY KEY,
    payment_id   TEXT NOT NULL REFERENCES payments(id),
    amount       NUMERIC(20,8) NOT NULL CHECK (amount > 0),
    reason       TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'pending'
                 CHECK (status IN ('pending','succeeded','failed')),
    external_id  TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_refunds_payment ON refunds(payment_id);

-- 用户端下单时可选：用户的一把客户 key 明文只在支付成功响应出现一次。
-- 这里只存哈希与前缀，和既有 api_keys 表口径一致。
-- （api_keys 已含 key_hash/key_prefix，无需新增字段。）
