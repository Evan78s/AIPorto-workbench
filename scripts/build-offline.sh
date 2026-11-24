#!/usr/bin/env bash
# 在【你自己的电脑】上运行：生成一个自包含的离线安装包 aiporto-install.sh
# 生成后只需把这一个文件传到服务器，执行 bash aiporto-install.sh 即可完成安装。
# 该安装包不依赖 GitHub：源码已内嵌在文件末尾。
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
WS="$(cd "$ROOT/.." && pwd)"
OUT="$WS/aiporto-install.sh"
STAGE="$WS/.offline-stage"

echo "项目目录：$ROOT"
echo "输出文件：$OUT"

# 0. 确认是 git 仓库，用 git ls-files 精确取被跟踪文件（自动排除 .env / node_modules / .git）
git -C "$ROOT" rev-parse --is-inside-work-tree >/dev/null 2>&1 || { echo "错误：项目不是 git 仓库"; exit 1; }
if git -C "$ROOT" ls-files | grep -qE '(^|/)\.env$'; then
  echo "错误：.env 被 git 跟踪了，拒绝打包（会泄露真实密钥）"
  exit 1
fi

# 1. 准备暂存目录
rm -rf "$STAGE"
mkdir -p "$STAGE/aiporto"
cd "$ROOT"
git ls-files -z | tar --null -T - -cf - | tar -xf - -C "$STAGE/aiporto"

# 2. 核对包内没有敏感文件
BAD="$(find "$STAGE/aiporto" -type f \( -name '.env' -o -name '*.key' -o -name '*.pem' -o -name 'seed*.txt' -o -name '*.gz' \) -print)"
if [ -n "$BAD" ]; then echo "错误：包内出现敏感文件："; echo "$BAD"; exit 1; fi

echo "包内文件数：$(find "$STAGE/aiporto" -type f | wc -l | tr -d ' ')"

# 3. 压缩并 base64 编码
tar -czf "$STAGE/payload.tar.gz" -C "$STAGE" aiporto
B64="$(base64 < "$STAGE/payload.tar.gz" | tr -d '\n')"
echo "压缩后大小：$(du -h "$STAGE/payload.tar.gz" | cut -f1)"

# 4. 生成自解压脚本：前半是安装逻辑，后半是内嵌源码
cat > "$OUT" <<'HEADER'
#!/usr/bin/env bash
# Aiporto Workbench 离线一键安装包（自解压）
# 用法：把本文件传到服务器任意目录，执行：bash aiporto-install.sh
# 说明：源码已内嵌，全程不访问 GitHub。只会从 Docker Hub 拉基础镜像。
set -euo pipefail

INSTALL_DIR=/www/wwwroot/aiporto
MARKER='__AIPORTO_ARCHIVE__'
SELF="$0"

echo "=========================================="
echo " Aiporto Workbench 离线一键安装"
echo " 安装目录：$INSTALL_DIR"
echo "=========================================="
echo ""

if [ ! -r "$SELF" ]; then
  echo "错误：读不到安装包自身（$SELF）。请用 bash 本文件名 的方式运行。"
  exit 1
fi

if [ -d "$INSTALL_DIR" ] && [ -n "$(ls -A "$INSTALL_DIR" 2>/dev/null)" ]; then
  echo "检测到 $INSTALL_DIR 已存在且非空。"
  echo "为避免覆盖已有数据，已停止。如确认重装，请先备份并移走："
  echo "  sudo mv $INSTALL_DIR ${INSTALL_DIR}.bak.\$(date +%Y%m%d)"
  exit 1
fi

# base64 解码兼容 macOS(BSD) 与 Linux(GNU)
b64_decode() {
  if printf 'Zg==' | base64 -d >/dev/null 2>&1; then
    base64 -d
  else
    base64 -D
  fi
}

echo "正在解压内嵌源码..."
mkdir -p "$INSTALL_DIR"
sed -n "/^${MARKER}\$/,\$p" "$SELF" | tail -n +2 | b64_decode | tar -xz -C "$(dirname "$INSTALL_DIR")"

if [ ! -f "$INSTALL_DIR/install.sh" ]; then
  echo "错误：解压失败，未找到 install.sh。安装包可能已损坏。"
  exit 1
fi
echo "解压完成：$(find "$INSTALL_DIR" -type f | wc -l | tr -d ' ') 个文件"

if [ "${AIPORTO_EXTRACT_ONLY:-}" = "1" ]; then
  echo "AIPORTO_EXTRACT_ONLY=1，只解压不安装（用于自检）。"
  exit 0
fi

echo ""
cd "$INSTALL_DIR"
bash install.sh
HEADER
printf '%s\n' '__AIPORTO_ARCHIVE__' >> "$OUT"
printf '%s\n' "$B64" >> "$OUT"
chmod +x "$OUT"

echo "完成：$OUT"
echo "大小：$(du -h "$OUT" | cut -f1)"
echo "传到服务器后执行：bash aiporto-install.sh"
