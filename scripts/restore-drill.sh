#!/usr/bin/env bash
# 演练恢复：把备份恢复到临时库并核对关键表行数，不碰生产库。
# 用法：./scripts/restore-drill.sh backups/wb_20261123_020000.dump.gz
set -euo pipefail
cd "$(dirname "$0")/.."

FILE="${1:-}"
if [ -z "$FILE" ] || [ ! -f "$FILE" ]; then
  echo "用法: $0 <备份文件.dump.gz>"
  exit 1
fi

COMPOSE="docker compose -f docker-compose.prod.yaml"
DB="wb_restore_drill"

echo "[1/4] 创建临时库 $DB"
$COMPOSE exec -T postgres sh -c "createdb -U \"\$POSTGRES_USER\" --if-exists $DB"

echo "[2/4] 恢复到临时库"
gunzip -c "$FILE" | $COMPOSE exec -T postgres sh -c \
  "pg_restore -U \"\$POSTGRES_USER\" -d $DB --clean --if-exists --no-owner --no-privileges"

echo "[3/4] 核对关键表行数"
$COMPOSE exec -T postgres sh -c "psql -U \"\$POSTGRES_USER\" -d $DB -c \
  \"SELECT (SELECT count(*) FROM tenants) AS tenants, (SELECT count(*) FROM usage_logs) AS usage_logs, (SELECT count(*) FROM audit_logs) AS audit_logs;\""

echo "[4/4] 删除临时库"
$COMPOSE exec -T postgres sh -c "dropdb -U \"\$POSTGRES_USER\" --if-exists $DB"

echo "演练完成。请把上面的行数与备份当天的记录对照。"
