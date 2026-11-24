#!/usr/bin/env bash
# 数据库每日备份：custom format + gzip，保留 30 天。
# 用法：./scripts/backup.sh   （建议由宿主机 crontab 每天 02:00 调用）
set -euo pipefail
cd "$(dirname "$0")/.."

COMPOSE="docker compose -f docker-compose.prod.yaml"
BACKUP_DIR="${BACKUP_DIR:-./backups}"
KEEP_DAYS="${KEEP_DAYS:-30}"
mkdir -p "$BACKUP_DIR"

STAMP="$(date +%Y%m%d_%H%M%S)"
OUT="$BACKUP_DIR/wb_${STAMP}.dump"

echo "[1/4] 导出数据库到 $OUT"
$COMPOSE exec -T postgres sh -c 'pg_dump -U "$POSTGRES_USER" -Fc -d "$POSTGRES_DB"' > "$OUT"

echo "[2/4] 校验备份完整性（pg_restore -l）"
$COMPOSE exec -T postgres sh -c 'pg_restore -l' < "$OUT" > /dev/null

echo "[3/4] 压缩"
gzip -f "$OUT"

echo "[4/4] 清理 ${KEEP_DAYS} 天前的旧备份"
find "$BACKUP_DIR" -maxdepth 1 -name 'wb_*.dump.gz' -type f -mtime "+${KEEP_DAYS}" -print -delete

echo "完成：${OUT}.gz"
