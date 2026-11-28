#!/usr/bin/env bash
# Aiporto 智港 · 从全量快照迁移到新服务器（也可用于本机灾难恢复）
# 用法：bash scripts/migrate.sh /路径/aiporto-full-YYYYMMDD_HHMMSS.tar.gz
#
# 前置：新服务器已 git clone 本仓库到目标目录（不需要先 install.sh，脚本会起容器）。
# 流程：校验快照 → 保护现有 .env → 恢复代码/.env → 恢复数据库 → 恢复 Redis → 重建并健康检查。
set -euo pipefail

cd "$(dirname "$0")/.."
SNAPSHOT="${1:-}"
if [ -z "$SNAPSHOT" ] || [ ! -f "$SNAPSHOT" ]; then
  echo "用法: $0 <aiporto-full-*.tar.gz>" >&2
  exit 1
fi
COMPOSE_FILE="docker-compose.prod.yaml"
COMPOSE="docker compose -f $COMPOSE_FILE"
STAMP="$(date +%Y%m%d_%H%M%S)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

[ -f "$COMPOSE_FILE" ] || { echo "找不到 $COMPOSE_FILE，请先 git clone 到本项目目录" >&2; exit 1; }
command -v docker >/dev/null 2>&1 || { echo "错误：缺少 docker 命令" >&2; exit 1; }
$COMPOSE version >/dev/null 2>&1 || { echo "错误：docker compose 不可用" >&2; exit 1; }

echo "⚠️  此操作会用快照覆盖当前代码、.env 和数据库，可能导致不可逆的数据丢失。"
echo "快照文件: $SNAPSHOT"
read -r -p "确认请输入 YES: " ans
if [ "$ans" != "YES" ]; then
  echo "已取消"
  exit 1
fi

echo "[1/7] 校验快照完整性..."
tar -xzf "$SNAPSHOT" -C "$TMP"
if ! ( cd "$TMP" && sha256sum -c SHA256SUMS ) >/dev/null 2>&1; then
  echo "错误：快照校验失败，文件可能损坏或被篡改" >&2
  exit 1
fi
echo "      校验通过。快照信息："
sed 's/^/      /' "$TMP/MANIFEST.txt"

echo "[2/7] 保护现有 .env..."
if [ -f .env ]; then
  cp -a .env ".env.pre-migrate-${STAMP}"
  echo "      已另存为 .env.pre-migrate-${STAMP}"
fi

echo "[3/7] 恢复工程文件与 .env..."
tar -cf - -C "$TMP/project" . | tar -xf - -C .
chmod 600 .env

echo "[4/7] 启动 PostgreSQL 与 Redis..."
$COMPOSE up -d --no-deps postgres redis
for _ in $(seq 1 36); do
  if $COMPOSE exec -T postgres sh -c 'pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB"' >/dev/null 2>&1; then
    break
  fi
  sleep 2
done
for _ in $(seq 1 36); do
  if $COMPOSE exec -T redis redis-cli ping >/dev/null 2>&1; then
    break
  fi
  sleep 2
done

echo "[5/7] 恢复 PostgreSQL 数据..."
$COMPOSE exec -T postgres sh -c \
  'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists --no-owner --no-privileges' \
  < "$TMP/db/aiporto.dump"

echo "[6/7] 恢复 Redis 数据..."
$COMPOSE stop redis >/dev/null
if $COMPOSE cp "$TMP/redis/dump.rdb" redis:/data/dump.rdb >/dev/null 2>&1; then
  echo "      Redis RDB 已恢复。"
else
  echo "      警告：Redis RDB 恢复失败，将使用空缓存启动（不影响业务数据）。"
fi
$COMPOSE start redis >/dev/null

echo "[7/7] 重建全部服务并健康检查..."
# --force-recreate 确保新 .env 生效；restart 不会重载环境变量。
$COMPOSE up -d --build --force-recreate --remove-orphans
OK=0
for _ in $(seq 1 40); do
  if curl -fsS --max-time 3 http://127.0.0.1:8080/healthz >/dev/null 2>&1; then
    OK=1
    break
  fi
  sleep 2
done
if [ "$OK" -ne 1 ]; then
  echo "错误：网关健康检查未通过，请查看：docker compose -f $COMPOSE_FILE logs --tail=100 gateway" >&2
  exit 1
fi
curl -fsS --max-time 3 http://127.0.0.1:8080/readyz || {
  echo "错误：readyz 未就绪，请检查数据库迁移状态" >&2
  exit 1
}

echo ""
echo "迁移完成。还需在新服务器手工完成（快照不包含）："
echo "  1. 域名解析指向新服务器 IP"
echo "  2. 宝塔/Nginx 配置 gw / admin / user 三个反代，并确保 proxy_buffering off;"
echo "  3. 申请 SSL 证书、开启 HTTPS"
echo "  4. 防火墙只放行 80/443，不要暴露 8080/8081/9090"
echo "请保持旧服务器运行，验证新站无误后再切换 DNS。"
