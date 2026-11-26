#!/usr/bin/env bash
# 上线前脱敏自检：静态扫描四类敏感模式。任何 FAIL 都不得上线。
set -uo pipefail
cd "$(dirname "$0")/.."
mkdir -p .toolcache
fail=0

check() {
  local title="$1"; shift
  echo "--- $title"
  if "$@" > ./.toolcache/redaction-hits 2>&1; then
    echo "PASS"
  else
    if [ -s ./.toolcache/redaction-hits ]; then
      cat ./.toolcache/redaction-hits
    fi
    echo "FAIL"
    fail=1
  fi
}

echo "===== 脱敏与红线自检 ====="

check "usage_logs 不存 prompt 原文列" \
  bash -c '! grep -RniE "(prompt_text|prompt_content|messages_json|request_body|raw_body)" migrations/ internal/metering/ internal/proxy/'

check "前端代码不含真实 key 或上游密文字段" \
  bash -c '! grep -RniE "(sk-wb-[a-f0-9]{8}|SEED_UPSTREAM_KEY_PLAIN|encrypted_key|UPSTREAM_MASTER_KEY)" frontend/src/'

check "日志语句不打印鉴权头或上游明文 key" \
  bash -c '! grep -RniE "log\.[A-Za-z]+\([^)]*(Authorization|PlainKey|plainUpstream|MasterKey)" internal/ cmd/'

check "审计 detail 只写元数据" \
  bash -c 'grep -q "AuditDetail" internal/consoleapi/audit.go && ! grep -A12 "func AuditDetail" internal/consoleapi/audit.go | grep -qiE "body|authorization"'

check "客户控制台 SQL 全部带租户过滤" \
  bash -c 'for q in consoleOverviewSQL consoleByModelSQL consoleDailySQL consoleKeysSQL consoleRechargesSQL consoleUsageExportSQL consoleBillExportSQL; do grep -A8 "^[[:space:]]*$q[[:space:]]*=" internal/consoleapi/api.go | grep -q "tenant_id = \$1" || exit 1; done'

check "管理后台已移除 Basic Auth" \
  bash -c '! grep -Rni "BasicAuth" internal/consoleapi/ frontend/src/'

check "网站根目录兜底防护配置存在且被文档引用" \
  bash -c 'test -s nginx-deny.conf && grep -q "deny all" nginx-deny.conf && grep -q "nginx-deny.conf" BAOTA-INSTALL.md && grep -q "nginx-deny.conf" INSTALL.md'

check "安装路径统一为 /www/docker/aiporto" \
  bash -c '! grep -Rqn "/opt/aip""ort" --include="*.md" --include="*.sh" --include="*.yaml" . --exclude-dir=node_modules --exclude-dir=.git --exclude-dir=.toolcache --exclude-dir=dist'

rm -f ./.toolcache/redaction-hits

if [ "$fail" -ne 0 ]; then
  echo "===== 存在 FAIL，禁止上线 ====="
  exit 1
fi
echo "===== 全部 PASS ====="
