#!/usr/bin/env bash
# Aiporto Workbench 服务器一键安装脚本
# 用法：在项目目录执行 bash install.sh
# 说明：只自动生成技术密钥（数据库/加密/TOTP），不生成客户 key，也不替你决定管理员账号。
set -euo pipefail

cd "$(dirname "$0")"
COMPOSE_FILE="docker-compose.prod.yaml"

echo "=========================================="
echo " Aiporto Workbench 一键安装"
echo "=========================================="

# 1. 检查基础环境
if [ "$(id -u)" -ne 0 ] && ! sudo -n true 2>/dev/null; then
  echo "提示：非 root 用户可能需要 sudo 权限来安装 Docker。"
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "[1/6] 未检测到 Docker，正在安装（使用阿里云镜像加速）..."
  curl -fsSL https://get.docker.com | sh -s -- --mirror Aliyun
else
  echo "[1/6] Docker 已存在，跳过安装。"
fi

if ! docker compose version >/dev/null 2>&1; then
  echo "错误：Docker Compose 插件不可用。请先安装 Docker Compose v2 后重试。"
  exit 1
fi

for tool in openssl python3; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "错误：缺少 $tool，请先安装后重试。"
    exit 1
  fi
done

# 2. 生成 .env（绝不覆盖已有配置）
if [ -f .env ]; then
  echo "[2/6] 检测到已存在的 .env，保留原配置，不覆盖。"
else
  echo "[2/6] 首次运行，正在生成 .env 与随机技术密钥..."

  export AIPORT_DB_NAME="aiport"
  export AIPORT_DB_USER="aiport"
  export AIPORT_DB_PASS="$(openssl rand -hex 20)"
  export AIPORT_PEPPER="$(openssl rand -base64 32)"
  export AIPORT_MASTER="$(openssl rand -base64 32)"
  export AIPORT_TOTP="$(python3 -c 'import base64,os;print(base64.b32encode(os.urandom(20)).decode().rstrip("="))')"

  python3 - <<'PY'
import os, re
src = open('.env.example', encoding='utf-8').read()
db, user, pwd = os.environ['AIPORT_DB_NAME'], os.environ['AIPORT_DB_USER'], os.environ['AIPORT_DB_PASS']
vals = {
    'POSTGRES_DB': db,
    'POSTGRES_USER': user,
    'POSTGRES_PASSWORD': pwd,
    'DATABASE_URL': f'postgres://{user}:{pwd}@postgres:5432/{db}?sslmode=disable',
    'API_KEY_PEPPER_B64': os.environ['AIPORT_PEPPER'],
    'UPSTREAM_MASTER_KEY_B64': os.environ['AIPORT_MASTER'],
    # Redis 容器未开启 requirepass，这里必须留空，否则网关连不上。
    'REDIS_PASSWORD': '',
    # 管理员账号由用户自己决定，脚本不代填、不生成。
    'ADMIN_USER': '',
    'ADMIN_PASSWORD': '',
    'ADMIN_TOTP_SECRET': os.environ['AIPORT_TOTP'],
}
out = []
for line in src.splitlines():
    m = re.match(r'^([A-Z][A-Z0-9_]*)=', line)
    if m and m.group(1) in vals:
        out.append(f"{m.group(1)}={vals[m.group(1)]}")
    else:
        out.append(line)
open('.env', 'w', encoding='utf-8').write('\n'.join(out) + '\n')
PY
  chmod 600 .env

  echo ""
  echo "以下凭据只显示这一次，请立即存进你自己的密码管理器："
  echo "  TOTP 私钥（录入验证器 App）： $AIPORT_TOTP"
  echo "  数据库账号/密码： $AIPORT_DB_USER / $AIPORT_DB_PASS"
  echo ""
  echo "客户 key 未生成；它会在你签发时明文显示一次（见后续步骤）。"
fi

# 3. 启动容器
echo "[3/6] 拉取并启动容器（首次构建可能需要几分钟）..."
docker compose -f "$COMPOSE_FILE" pull
docker compose -f "$COMPOSE_FILE" up -d --build

# 4. 等待健康检查通过
echo "[4/6] 等待网关健康检查通过（最多 3 分钟）..."
cid=""
ready=0
for _ in $(seq 1 36); do
  cid="$(docker compose -f "$COMPOSE_FILE" ps -q gateway 2>/dev/null || true)"
  if [ -n "$cid" ]; then
    status="$(docker inspect -f '{{.State.Health.Status}}' "$cid" 2>/dev/null || echo starting)"
    if [ "$status" = "healthy" ]; then
      ready=1
      break
    fi
  fi
  sleep 5
done

if [ "$ready" -ne 1 ]; then
  echo "错误：网关未在 3 分钟内达到 healthy。请查看日志："
  echo "  docker compose -f $COMPOSE_FILE logs --tail=100 gateway"
  exit 1
fi
echo "网关健康检查通过。"

# 5. 提示仍需用户决定的配置
echo "[5/6] 检查还需你亲自填写的配置..."
missing=""
for key in ADMIN_USER ADMIN_PASSWORD ADMIN_TOTP_SECRET; do
  val="$(grep -E "^${key}=" .env | head -1 | cut -d= -f2- || true)"
  if [ -z "$val" ] || echo "$val" | grep -q 'REPLACE_'; then
    missing="$missing $key"
  fi
done
if [ -n "$missing" ]; then
  echo "请在 .env 中填上：$missing"
  echo "填完后执行：docker compose -f $COMPOSE_FILE restart gateway"
fi

# 6. 输出访问信息
echo "[6/6] 安装完成。"
echo ""
echo "  管理后台 / 客户控制台 : http://服务器IP:8081"
echo "  模型 API              : http://服务器IP:8080/v1"
echo "  监控（Prometheus）    : http://服务器IP:9090"
echo ""
echo "安全提醒：8080/8081/9090 不要直接暴露公网。请配反向代理 + HTTPS，"
echo "或在防火墙只放行 80/443。详见 BAOTA-INSTALL.md。"
echo ""
echo "下一步：录入厂商 key、建套餐与租户，见 SERVER-INSTALL.md 第 7-8 节。"
