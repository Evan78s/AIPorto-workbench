#!/usr/bin/env bash
# Aiporto 智港 · 一条命令全自动升级
# 用法：cd /www/docker/aiporto && bash scripts/update.sh
#
# 流程：
#   0. 前置检查（目录/.env/compose/git/远程）
#   1. 记录当前版本，用于失败回滚
#   2. 备份数据库（含完整性校验）
#   3. git pull 拉取新代码
#   4. 幂等补跑全部数据库迁移
#   5. 重建 gateway/admin-ui/user-ui 镜像与容器
#   6. 健康检查（容器 healthy + 网关 /healthz）
#   7. 成功：打印新版本；失败：回滚代码到旧版本并重建，数据库若已迁移则提示用备份恢复
#
# 安全说明：本脚本只在宿主机执行，不授予容器任何 Docker 权限。
set -euo pipefail

ROOT="/www/docker/aiporto"
COMPOSE_FILE="${ROOT}/docker-compose.prod.yaml"
LOG="${ROOT}/backups/update.log"

cd "$ROOT" || { echo "找不到 $ROOT，请用 git clone 安装到该目录" >&2; exit 1; }
mkdir -p "${ROOT}/backups"

log() { printf '%s %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*" | tee -a "$LOG"; }
fail() { log "错误：$*"; exit 1; }

log "========== 开始升级 =========="

# ---------- 0. 前置检查 ----------
[ -f "$COMPOSE_FILE" ] || fail "找不到 $COMPOSE_FILE"
[ -f "${ROOT}/.env" ] || fail "找不到 ${ROOT}/.env"
command -v docker >/dev/null 2>&1 || fail "本机没有 docker 命令"
docker compose version >/dev/null 2>&1 || fail "docker compose 不可用"
command -v git >/dev/null 2>&1 || fail "本机没有 git 命令"
git rev-parse --is-inside-work-tree >/dev/null 2>&1 || fail "当前目录不是 git 仓库"

# 检查本地是否有未提交改动：有则中止，避免 git pull 冲突覆盖你的修改
if [ -n "$(git status --porcelain --untracked-files=no)" ]; then
  fail "检测到本地有未提交的改动。请先提交或备份后再升级：git status 查看"
fi

OLD_COMMIT="$(git rev-parse HEAD)"
OLD_SHORT="$(git rev-parse --short HEAD)"
log "当前版本：$OLD_SHORT"

# ---------- 1. 备份 ----------
log "[1/6] 备份数据库..."
if ! bash "${ROOT}/scripts/backup.sh" >>"$LOG" 2>&1; then
  fail "数据库备份失败，已中止升级（没有备份不升级）"
fi
LATEST_BACKUP="$(ls -1t "${ROOT}"/backups/wb_*.dump.gz 2>/dev/null | head -1)"
[ -n "$LATEST_BACKUP" ] || fail "备份完成后找不到备份文件，已中止"
log "备份完成：$(basename "$LATEST_BACKUP")"

# ---------- 2. 拉取代码 ----------
log "[2/6] 拉取新代码..."
if ! git pull --ff-only >>"$LOG" 2>&1; then
  fail "git pull 失败（可能有本地改动或远端冲突），未做任何更改"
fi
NEW_COMMIT="$(git rev-parse HEAD)"
NEW_SHORT="$(git rev-parse --short HEAD)"
if [ "$OLD_COMMIT" = "$NEW_COMMIT" ]; then
  log "已是最新版本（$NEW_SHORT），跳过重建"
  log "========== 升级完成（无更新） =========="
  exit 0
fi
log "新版本：$NEW_SHORT"

# ---------- 3. 数据库迁移 ----------
log "[3/6] 补跑数据库迁移..."
if ! bash "${ROOT}/scripts/migrate-all.sh" >>"$LOG" 2>&1; then
  log "迁移失败，回滚代码到 $OLD_SHORT..."
  git checkout -q "$OLD_COMMIT" 2>/dev/null || true
  fail "迁移失败。代码已回滚；数据库可能部分变更，必要时用备份恢复：bash scripts/restore.sh $(basename "$LATEST_BACKUP")"
fi

# ---------- 4. 重建容器 ----------
# 注意：用 --force-recreate 确保 .env 变更生效（restart 不会重载环境变量）
log "[4/6] 重建容器（gateway / admin-ui / user-ui）..."
if ! docker compose -f "$COMPOSE_FILE" up -d --build --force-recreate --remove-orphans \
    gateway admin-ui user-ui >>"$LOG" 2>&1; then
  log "容器重建失败，回滚代码到 $OLD_SHORT 并重建..."
  git checkout -q "$OLD_COMMIT" 2>/dev/null || true
  docker compose -f "$COMPOSE_FILE" up -d --build --force-recreate gateway admin-ui user-ui >>"$LOG" 2>&1 || true
  fail "容器重建失败，已回滚代码。请查看日志：$LOG"
fi

# ---------- 5. 健康检查 ----------
log "[5/6] 健康检查..."
HEALTHY=0
for i in $(seq 1 30); do
  if curl -fsS --max-time 3 http://127.0.0.1:8080/healthz >/dev/null 2>&1; then
    HEALTHY=1
    break
  fi
  sleep 2
done
if [ "$HEALTHY" -ne 1 ]; then
  log "网关 30 次健康检查未通过，回滚代码到 $OLD_SHORT 并重建..."
  git checkout -q "$OLD_COMMIT" 2>/dev/null || true
  docker compose -f "$COMPOSE_FILE" up -d --build --force-recreate gateway admin-ui user-ui >>"$LOG" 2>&1 || true
  fail "健康检查失败，已回滚代码。请查看日志：$LOG，必要时恢复备份"
fi
UNHEALTHY="$(docker compose -f "$COMPOSE_FILE" ps --format '{{.Name}} {{.Status}}' 2>/dev/null | grep -i 'unhealthy' || true)"
if [ -n "$UNHEALTHY" ]; then
  log "警告：以下容器状态异常：$UNHEALTHY"
fi

# ---------- 6. 完成 ----------
log "[6/6] 升级完成：$OLD_SHORT -> $NEW_SHORT"
log "备份文件：$(basename "$LATEST_BACKUP")"
log "如果新版本有问题，回滚命令："
log "  cd $ROOT && git checkout $OLD_COMMIT && docker compose -f $COMPOSE_FILE up -d --build --force-recreate gateway admin-ui user-ui"
log "========== 升级成功 =========="
