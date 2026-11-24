#!/usr/bin/env bash
# 从备份恢复生产库（覆盖当前数据，操作不可逆）。
# 用法：./scripts/restore.sh backups/wb_20261123_020000.dump.gz
set -euo pipefail
cd "$(dirname "$0")/.."

FILE="${1:-}"
if [ -z "$FILE" ] || [ ! -f "$FILE" ]; then
  echo "用法: $0 <备份文件.dump.gz>"
  exit 1
fi

echo "⚠️  此操作会覆盖当前数据库，可能导致不可逆的数据丢失。"
echo "影响文件: $FILE"
read -r -p "确认请输入 YES: " ans
if [ "$ans" != "YES" ]; then
  echo "已取消"
  exit 1
fi

COMPOSE="docker compose -f docker-compose.prod.yaml"
echo "恢复中..."
gunzip -c "$FILE" | $COMPOSE exec -T postgres sh -c \
  'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists --no-owner --no-privileges'

echo "恢复完成。立即验证：curl -fsS http://127.0.0.1:8080/readyz"
