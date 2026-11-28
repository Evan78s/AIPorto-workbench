#!/usr/bin/env bash
# 只读诊断：为什么服务器上找不到 scripts/update.sh / 升级失败。
# 用法：cd /www/docker/aiporto && bash scripts/diagnose-update.sh
set -u
cd "$(dirname "$0")/.." || exit 1
ROOT="$(pwd)"
echo "当前目录: $ROOT"
echo "================================"

echo "[1] 目录与关键文件"
for f in .env docker-compose.prod.yaml scripts/update.sh scripts/backup.sh; do
  if [ -f "$f" ]; then echo "  存在  $f"; else echo "  缺失  $f"; fi
done

echo "[2] 是否 git 仓库"
if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "  是 git 仓库"
  echo "  当前版本: $(git rev-parse --short HEAD)"
  echo "  当前分支: $(git branch --show-current)"
  echo "  未提交改动数: $(git status --porcelain | wc -l | tr -d ' ')"
  echo "  最近 3 条提交:"
  git log --oneline -3 | sed 's/^/    /'
  echo "  远程地址: $(git remote get-url origin 2>/dev/null || echo '未配置')"
else
  echo "  不是 git 仓库（说明不是 git clone 安装的，update.sh 依赖 git pull，无法用）"
fi

echo "[3] 容器状态"
docker compose -f docker-compose.prod.yaml ps --format 'table {{.Name}}\t{{.Status}}' 2>&1 | sed 's/^/  /'

echo "[4] 网关版本接口"
curl -fsS --max-time 5 http://127.0.0.1:8080/api/admin/system/version 2>/dev/null | head -c 300 || echo "  未登录或接口不可达（需后台会话令牌，属正常）"

echo "================================"
echo "处理建议："
echo "  A. 如果 [2] 显示「不是 git 仓库」→ 你是用离线包/手工拷贝安装的，没有版本管理。"
echo "     需要先重新用 git 安装（会保留现有 .env 和数据库）："
echo "       cd /www && mv docker/aiporto docker/aiporto.bak-\$(date +%Y%m%d)"
echo "       git clone https://github.com/Evan78s/AIPorto-workbench.git /www/docker/aiporto"
echo "       cp /www/docker/aiporto.bak-*/.env /www/docker/aiporto/.env"
echo "       cd /www/docker/aiporto && docker compose -f docker-compose.prod.yaml up -d --build"
echo "     确认无误后再删除 .bak 目录。数据库在 Docker 卷里，不受影响。"
echo "  B. 如果是 git 仓库但版本落后 → git pull --ff-only 后再跑 upgrade。"
echo "  C. 如果 git pull 因本地改动失败 → 先备份 .env，再 git stash，pull 完 stash pop。"
