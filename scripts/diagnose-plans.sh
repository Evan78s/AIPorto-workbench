#!/usr/bin/env bash
# 诊断「用户端看不到可售套餐」：只读，不修改任何数据。
# 用法：cd /www/docker/aiporto && bash scripts/diagnose-plans.sh
set -u
cd /www/docker/aiporto || exit 1
COMPOSE="docker compose -f /www/docker/aiporto/docker-compose.prod.yaml"

echo "==================== 1. 代码版本 ===================="
echo "本地 HEAD: $(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
echo "远程 HEAD: $(git ls-remote origin refs/heads/main 2>/dev/null | awk '{print substr($1,1,7)}' || echo unknown)"
echo "（两者不同 = 还没 git pull，后台新增套餐表单可能不存在）"

echo
echo "==================== 2. 容器状态 ===================="
$COMPOSE ps --format "table {{.Name}}\t{{.Status}}\t{{.Ports}}" 2>&1 | head -20

echo
echo "==================== 3. 数据库里的套餐 ===================="
set -a
# shellcheck disable=SC1091
. ./.env 2>/dev/null
set +a
$COMPOSE exec -T postgres psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-aiporto}" \
  -c "SELECT code, name, price_per_month, currency, created_at FROM plans ORDER BY created_at DESC;" 2>&1 | head -30
echo "（空表 = 套餐没写进去；price_per_month=0 = 用户端故意不显示）"

echo
echo "==================== 4. 用户端实际拿到的套餐 ===================="
echo "--- 经 user-ui 反代（模拟浏览器）---"
curl -sS --max-time 10 -i http://127.0.0.1:8082/api/user/plans 2>&1 | head -25
echo
echo "--- 直连 gateway ---"
curl -sS --max-time 10 -i http://127.0.0.1:8080/api/user/plans 2>&1 | head -25

echo
echo "==================== 5. 判断提示 ===================="
echo "A. 第3步价格全为 0 或表为空  -> 后台套餐价格填了 0，或根本没保存成功"
echo "B. 第3步有 >0 但第4步返回 [] -> 用户端反代/网关缓存问题，或连的不是同一个库"
echo "C. 第4步两个都返回套餐      -> 服务器没问题，是你浏览器缓存，强制刷新(Ctrl+F5)"
echo "D. 第1步本地≠远程           -> 先 git pull 并重建 admin-ui，才有新增套餐表单"
