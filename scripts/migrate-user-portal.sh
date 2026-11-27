#!/usr/bin/env bash
# 给「已有数据」的旧库补跑 004_user_portal.sql。
# docker-entrypoint-initdb.d 只在首次初始化空库时执行，升级已有库必须手动跑本脚本。
# 迁移 SQL 全部用 CREATE TABLE IF NOT EXISTS / ADD COLUMN IF NOT EXISTS，重复执行安全。
set -euo pipefail

DIR="$(cd "$(dirname "$0")/.." && pwd)"
COMPOSE="${DIR}/docker-compose.prod.yaml"
ENV_FILE="${DIR}/.env"

if [ ! -f "$ENV_FILE" ]; then
  echo "找不到 $ENV_FILE，请先配置 .env" >&2
  exit 1
fi

# 从 .env 读取数据库账号（不 source 整个文件，避免特殊字符被 shell 解释）。
DB_USER="$(grep -E '^POSTGRES_USER=' "$ENV_FILE" | head -1 | cut -d= -f2- || true)"
DB_NAME="$(grep -E '^POSTGRES_DB=' "$ENV_FILE" | head -1 | cut -d= -f2- || true)"
DB_USER="${DB_USER:-aiporto}"
DB_NAME="${DB_NAME:-aiporto}"

echo "==> 对数据库 ${DB_NAME}（用户 ${DB_USER}）执行 004_user_portal.sql"
docker compose -f "$COMPOSE" exec -T postgres \
  psql -U "$DB_USER" -d "$DB_NAME" -v ON_ERROR_STOP=1 \
  < "${DIR}/migrations/004_user_portal.sql"

echo "==> 校验新表是否创建成功"
docker compose -f "$COMPOSE" exec -T postgres \
  psql -U "$DB_USER" -d "$DB_NAME" -c \
  "SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename IN ('users','email_tokens','user_sessions','orders','payments','refunds') ORDER BY tablename;"

echo "完成。若看到 6 张表即迁移成功。"
