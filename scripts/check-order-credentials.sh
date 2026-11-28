#!/usr/bin/env bash
# 只读诊断：查看指定订单的凭据状态，判断"领取密钥"报错的原因。
# 用法：bash scripts/check-order-credentials.sh [订单号]
# 不传订单号时列出最近 20 笔订单的凭据状态。
# 注意：不 source .env（SMTP_FROM 含 < > 会被 shell 当语法错误），改用 grep 精确读取所需变量。
set -euo pipefail
cd "$(dirname "$0")/.."
COMPOSE_FILE="docker-compose.prod.yaml"
ENV_FILE="./.env"

env_get() {
  local key="$1" default="${2:-}" val
  val="$(grep -m1 "^${key}=" "$ENV_FILE" 2>/dev/null || true)"
  val="${val#*=}"
  val="${val#\"}"; val="${val%\"}"
  val="${val#\'}"; val="${val%\'}"
  val="$(printf '%s' "$val" | tr -d '\r')"
  printf '%s' "${val:-$default}"
}

PG_USER="$(env_get POSTGRES_USER aiporto)"
PG_DB="$(env_get POSTGRES_DB aiporto)"
NO="${1:-}"

if [ -n "$NO" ]; then
  SQL="
  SELECT o.order_no, u.email, o.status, o.granted_key AS 已开通,
         (o.pending_credentials IS NOT NULL AND length(o.pending_credentials)>0) AS 待领取明文,
         o.credentials_taken_at AS 领取时间, o.tenant_id AS 租户ID,
         CASE
           WHEN o.status<>'paid' THEN '未支付'
           WHEN o.pending_credentials IS NOT NULL AND length(o.pending_credentials)>0 THEN '可领取'
           WHEN o.credentials_taken_at IS NOT NULL THEN '已领取（明文已清空）'
           ELSE '凭据未生成'
         END AS 结论
  FROM orders o LEFT JOIN users u ON u.id=o.user_id
  WHERE o.order_no='$NO';"
else
  SQL="
  SELECT o.order_no, u.email, o.status, o.granted_key AS 已开通,
         (o.pending_credentials IS NOT NULL AND length(o.pending_credentials)>0) AS 待领取明文,
         o.credentials_taken_at AS 领取时间, o.tenant_id AS 租户ID,
         CASE
           WHEN o.status<>'paid' THEN '未支付'
           WHEN o.pending_credentials IS NOT NULL AND length(o.pending_credentials)>0 THEN '可领取'
           WHEN o.credentials_taken_at IS NOT NULL THEN '已领取（明文已清空）'
           ELSE '凭据未生成'
         END AS 结论
  FROM orders o LEFT JOIN users u ON u.id=o.user_id
  ORDER BY o.created_at DESC LIMIT 20;"
fi

docker compose -f "$COMPOSE_FILE" exec -T postgres \
  psql -U "$PG_USER" -d "$PG_DB" -P pager=off -c "$SQL"
