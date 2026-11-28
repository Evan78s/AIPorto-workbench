#!/usr/bin/env bash
# 幂等执行全部数据库迁移：给已有数据库补跑新增表/字段。
# docker-entrypoint-initdb.d 只在首次初始化空库时执行，升级已有库必须手动跑本脚本。
# 所有迁移 SQL 均用 CREATE TABLE IF NOT EXISTS / ADD COLUMN IF NOT EXISTS，重复执行安全。
# 用法：bash scripts/migrate-all.sh   （update.sh 会自动调用）
set -euo pipefail

DIR="$(cd "$(dirname "$0")/.." && pwd)"
COMPOSE_FILE="${DIR}/docker-compose.prod.yaml"
ENV_FILE="${DIR}/.env"

if [ ! -f "$ENV_FILE" ]; then
  echo "找不到 $ENV_FILE，请先配置 .env" >&2
  exit 1
fi

# 不 source .env：SMTP_FROM 等值含 < > 空格时 shell 会报 syntax error，
# 导致迁移脚本在读取配置阶段就失败（旧版把错误吞掉，只显示"迁移失败"）。
# 只用 grep 精确读取迁移需要的两个变量。
env_get() {
  local key="$1" default="$2" val
  val="$(grep -m1 "^${key}=" "$ENV_FILE" 2>/dev/null || true)"
  val="${val#*=}"
  val="${val#\"}"; val="${val%\"}"
  val="${val#\'}"; val="${val%\'}"
  val="$(printf '%s' "$val" | tr -d '\r')"
  printf '%s' "${val:-$default}"
}

PG_USER="$(env_get POSTGRES_USER aiporto)"
PG_DB="$(env_get POSTGRES_DB aiporto)"

echo "[migrate] 等待 postgres 就绪..."
for i in $(seq 1 30); do
  if docker compose -f "$COMPOSE_FILE" exec -T postgres pg_isready -U "$PG_USER" -d "$PG_DB" >/dev/null 2>&1; then
    break
  fi
  if [ "$i" -eq 30 ]; then
    echo "[migrate] postgres 30 秒内未就绪，终止" >&2
    exit 1
  fi
  sleep 1
done

shopt -s nullglob
MIGRATIONS=("$DIR"/migrations/*.sql)
if [ "${#MIGRATIONS[@]}" -eq 0 ]; then
  echo "[migrate] 没有找到迁移文件，跳过"
  exit 0
fi

for file in "${MIGRATIONS[@]}"; do
  name="$(basename "$file")"
  echo "[migrate] 应用 $name ..."
  # 不再吞掉 SQL 错误：真实报错会被 update.sh 写进 backups/update.log，便于定位
  if ! docker compose -f "$COMPOSE_FILE" exec -T postgres \
      psql -v ON_ERROR_STOP=1 -U "$PG_USER" -d "$PG_DB" -f - < "$file"; then
    echo "[migrate] $name 执行失败（上方即为 PostgreSQL 真实报错）" >&2
    exit 1
  fi
done

echo "[migrate] 全部迁移已应用（共 ${#MIGRATIONS[@]} 个文件）"
