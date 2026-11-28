#!/usr/bin/env bash
# Aiporto 智港 · 一键全量快照（数据库 + Redis + .env + 工程代码）
# 用法：cd /www/docker/aiporto && bash scripts/snapshot.sh
#
# 产出：snapshots/aiporto-full-YYYYMMDD_HHMMSS.tar.gz
# 快照包含 .env（数据库密码、主密钥、支付私钥、TOTP 私钥），权限 600。
# 务必离线保存，不要上传到公开仓库。
set -euo pipefail

cd "$(dirname "$0")/.."
ROOT="$(pwd)"
COMPOSE_FILE="docker-compose.prod.yaml"
COMPOSE="docker compose -f $COMPOSE_FILE"
OUT_DIR="${SNAPSHOT_DIR:-$ROOT/snapshots}"
KEEP_DAYS="${KEEP_DAYS:-30}"
STAMP="$(date +%Y%m%d_%H%M%S)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

[ -f "$COMPOSE_FILE" ] || { echo "找不到 $COMPOSE_FILE，请在项目根目录执行" >&2; exit 1; }
[ -f .env ] || { echo "找不到 .env：没有它迁移后所有密钥都会失配" >&2; exit 1; }
command -v docker >/dev/null 2>&1 || { echo "错误：缺少 docker 命令" >&2; exit 1; }
$COMPOSE version >/dev/null 2>&1 || { echo "错误：docker compose 不可用" >&2; exit 1; }

mkdir -p "$TMP/project" "$TMP/db" "$TMP/redis" "$OUT_DIR"

echo "[1/6] 导出 PostgreSQL 全量备份..."
$COMPOSE exec -T postgres sh -c 'pg_dump -U "$POSTGRES_USER" -Fc -d "$POSTGRES_DB"' > "$TMP/db/aiporto.dump"
$COMPOSE exec -T postgres sh -c 'pg_restore -l' < "$TMP/db/aiporto.dump" > /dev/null
echo "      数据库导出与完整性校验通过。"

echo "[2/6] 导出 Redis 快照..."
$COMPOSE exec -T redis redis-cli SAVE >/dev/null
$COMPOSE cp redis:/data/dump.rdb "$TMP/redis/dump.rdb" >/dev/null
echo "      Redis RDB 已导出（限流计数、缓存；非必需但一并带走）。"

echo "[3/6] 复制工程文件与 .env..."
tar -cf - \
  --exclude=.git \
  --exclude=node_modules \
  --exclude=dist \
  --exclude=backups \
  --exclude=snapshots \
  --exclude=.toolcache \
  --exclude='*.log' \
  . | tar -xf - -C "$TMP/project"
chmod 600 "$TMP/project/.env"

echo "[4/6] 写入快照清单与校验和..."
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
{
  echo "Aiporto 智港 全量快照"
  echo "生成时间: $(date '+%Y-%m-%d %H:%M:%S %Z')"
  echo "代码版本: $COMMIT"
  echo "包含: PostgreSQL 全量 dump / Redis RDB / .env / 工程代码与配置"
  echo "新服务器恢复命令: bash scripts/migrate.sh <本快照文件>"
  echo "注意: 本快照包含数据库密码、主密钥、支付私钥，请离线保存。"
} > "$TMP/MANIFEST.txt"
( cd "$TMP" && find . -type f -exec sha256sum {} + ) > "$TMP/SHA256SUMS"

echo "[5/6] 打包..."
ARCHIVE="$OUT_DIR/aiporto-full-${STAMP}.tar.gz"
tar -czf "$ARCHIVE" -C "$TMP" .
chmod 600 "$ARCHIVE"

echo "[6/6] 清理 ${KEEP_DAYS} 天前的旧快照..."
find "$OUT_DIR" -maxdepth 1 -name 'aiporto-full-*.tar.gz' -type f -mtime "+$KEEP_DAYS" -print -delete

echo ""
echo "完成：$ARCHIVE"
echo "大小：$(du -h "$ARCHIVE" | cut -f1)"
echo "异地保存示例：scp '$ARCHIVE' root@备份服务器:/backup/"
