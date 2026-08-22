#!/usr/bin/env bash
# =============================================================================
# zhihuDP 快速部署到云服务器（解决服务器上装Go/拉代码/编译慢的问题）
# -----------------------------------------------------------------------------
# 原理：在本地交叉编译 Linux 二进制（单文件约 21M，前端已内嵌），
#       只 scp 上传一个文件，服务器跳过 装Go/拉代码/编译，秒级完成部署。
#
# 用法（在本机执行）：
#   ./upload.sh root@<服务器IP> [安装目录]
#
# 注意：本地交叉编译的是【当前 checkout 的分支】——
#   部署情绪版（含知乎情绪分析）：git checkout feature/emotion 后再执行本脚本
#   部署 main 版：git checkout main 后再执行本脚本
#
# 可选环境变量：
#   ARCH      目标架构 amd64（默认）/ arm64
#   APP_PORT  端口（默认 8080）
#   密钥等其余参数与 install.sh 一致，可追加在远程命令里：
#   DEEPSEEK_API_KEY=... ZHIHU_ACCESS_SECRET=... ./upload.sh root@<IP>
# =============================================================================
set -euo pipefail

SERVER="${1:?用法: ./upload.sh root@<服务器IP> [安装目录]}"
APP_PORT="${APP_PORT:-8080}"
ARCH="${ARCH:-amd64}"
CUR_BRANCH="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)"

C_RED='\033[0;31m'; C_GRN='\033[0;32m'; C_NC='\033[0m'
info() { echo -e "${C_GRN}[upload]${C_NC} $*"; }
fail() { echo -e "${C_RED}[upload] 错误:${C_NC} $*" >&2; exit 1; }

# ---------- 1. 本地交叉编译 ----------
info "本地交叉编译 linux/${ARCH} 二进制（当前分支: ${CUR_BRANCH}）..."
GOOS=linux GOARCH="$ARCH" CGO_ENABLED=0 go build \
  -trimpath -ldflags "-s -w" \
  -o /tmp/zhihudp.bin ./cmd/server \
  || fail "交叉编译失败（本机需 Go ≥ go.mod 要求）"
BIN_SIZE="$(du -h /tmp/zhihudp.bin | awk '{print $1}')"
info "编译完成：/tmp/zhihudp.bin（${BIN_SIZE}，单文件）"

# ---------- 2. 上传二进制 + 部署脚本 ----------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
info "上传二进制与部署脚本到 ${SERVER}..."
scp -q /tmp/zhihudp.bin "${SERVER}:/tmp/zhihudp.bin"
scp -q "${SCRIPT_DIR}/install.sh" "${SERVER}:/tmp/zhihudp-install.sh"
rm -f /tmp/zhihudp.bin

# ---------- 3. 远程快速部署（跳过装Go/拉代码/编译） ----------
info "远程执行快速部署（复用 install.sh，检测到预编译二进制自动跳过构建）..."
# 透传密钥环境变量（如有）
EXTRA=""
[[ -n "${DEEPSEEK_API_KEY:-}" ]] && EXTRA="${EXTRA} DEEPSEEK_API_KEY='${DEEPSEEK_API_KEY}'"
[[ -n "${ZHIHU_ACCESS_SECRET:-}" ]] && EXTRA="${EXTRA} ZHIHU_ACCESS_SECRET='${ZHIHU_ACCESS_SECRET}'"
[[ -n "${APP_PORT:-}" ]] && EXTRA="${EXTRA} APP_PORT=${APP_PORT}"

ssh -t "$SERVER" "APP_BIN=/tmp/zhihudp.bin ${EXTRA} bash /tmp/zhihudp-install.sh; rm -f /tmp/zhihudp.bin /tmp/zhihudp-install.sh"
info "完成。访问 http://<服务器公网IP>:${APP_PORT}"
