-- 安全红线补丁：所有写操作审计 + 管理后台双因素登录所需的防重放表。
-- 本文件不含任何演示数据、账号或密钥。

-- 修复既有缺口：停用/轮换 key 的 handler 会写这一列，001 建表脚本里漏了。
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS revoked_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS audit_logs (
    id           BIGSERIAL PRIMARY KEY,
    request_id   TEXT NOT NULL DEFAULT '',
    actor        TEXT NOT NULL DEFAULT 'anonymous',
    actor_ip     TEXT NOT NULL DEFAULT '',
    user_agent   TEXT NOT NULL DEFAULT '',
    action       TEXT NOT NULL,
    target_type  TEXT NOT NULL DEFAULT '',
    target_id    TEXT NOT NULL DEFAULT '',
    tenant_id    TEXT,
    detail       JSONB NOT NULL DEFAULT '{}'::jsonb,
    status_code  INT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_audit_created_at ON audit_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_tenant ON audit_logs(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_actor ON audit_logs(actor, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_logs(action, created_at DESC);

-- TOTP 每个时间步只能使用一次，防止 30 秒窗口内重放。
CREATE TABLE IF NOT EXISTS admin_totp_used (
    username   TEXT NOT NULL,
    step       BIGINT NOT NULL,
    used_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (username, step)
);
-- 只保留最近 1 天的重放记录即可，可由运维按需清理:
-- DELETE FROM admin_totp_used WHERE used_at < now() - interval '1 day';
