#!/usr/bin/env bash
# =============================================================================
# zhihuDP 一键部署脚本（空云服务器 / 全新 Linux）
# -----------------------------------------------------------------------------
# 用法（在云服务器上以 root 或 sudo 执行）：
#
#   方式 A：环境变量提供密钥（推荐，明文不落盘，直接生成密文）
#     DEEPSEEK_API_KEY="sk-xxx" \
#     ZHIHU_ACCESS_SECRET="xxx" \
#       bash install.sh
#
#   方式 B：交互输入密钥（终端不回显）
#     bash install.sh
#
#   方式 C：不填密钥，部署后打开网页在开屏弹窗上传（同样加密持久化）
#
# 可选环境变量：
#   APP_SOURCE  代码来源：git 仓库地址（默认 https://github.com/ZZ2416/zhihuDP.git）
#                或本地目录路径（把代码 rsync 上传，适合私有仓库）
#   GIT_BRANCH  克隆分支（默认仓库默认分支 main；emotion 版设 feature/emotion）
#   GO_VERSION  Go 版本（默认 1.25.5）
#   APP_PORT    监听端口（默认 8080，systemd + 防火墙同步）
#   APP_USER    运行用户（默认 zhihudp，非 root）
#   APP_DIR     安装目录（默认 /opt/zhihudp）
#   APP_BIN     预编译二进制路径（本地交叉编译后上传，跳过装 Go/拉代码/编译）
#   GOPROXY     Go 模块代理（默认 https://goproxy.cn,direct，加速依赖下载）
#   GO_DL_BASE  Go 工具链下载源（默认 https://go.dev/dl；国内慢可换
#               https://mirrors.aliyun.com/golang 或 https://golang.google.cn/dl）
# =============================================================================
set -euo pipefail

# ---------- 参数 ----------
APP_SOURCE="${APP_SOURCE:-https://github.com/ZZ2416/zhihuDP.git}"
GIT_BRANCH="${GIT_BRANCH:-}"
GO_VERSION="${GO_VERSION:-1.25.5}"
APP_PORT="${APP_PORT:-8080}"
APP_USER="${APP_USER:-zhihudp}"
APP_DIR="${APP_DIR:-/opt/zhihudp}"
APP_BIN="${APP_BIN:-/tmp/zhihudp.bin}"
CONFIG_PATH="${APP_DIR}/config.yaml"
PRIVATE_KEY="${APP_DIR}/.zhihudp/zhihudp_private.pem"

# 预编译二进制已上传 → 跳过 装Go/拉代码/编译（上传慢时的最快路径）
SKIP_BUILD=0
if [[ -f "$APP_BIN" && "${SKIP_BUILD_FORCE:-0}" != "1" ]]; then
  info "检测到预编译二进制 ${APP_BIN}，跳过 装Go/拉代码/编译 步骤"
  SKIP_BUILD=1
fi

C_RED='\033[0;31m'; C_GRN='\033[0;32m'; C_YLW='\033[1;33m'; C_NC='\033[0m'
info() { echo -e "${C_GRN}[deploy]${C_NC} $*"; }
warn() { echo -e "${C_YLW}[deploy]${C_NC} $*"; }
fail() { echo -e "${C_RED}[deploy] 错误:${C_NC} $*" >&2; exit 1; }

[[ $EUID -eq 0 ]] || fail "请用 root 或 sudo 执行本脚本"

# ---------- 0. 系统检测 ----------
OS_ID="$(. /etc/os-release 2>/dev/null && echo "${ID:-unknown}")"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  GO_ARCH="amd64" ;;
  aarch64) GO_ARCH="arm64" ;;
  *) fail "不支持的架构: $ARCH（仅 amd64 / arm64）" ;;
esac
info "系统: ${OS_ID} / ${ARCH}，端口 ${APP_PORT}，安装目录 ${APP_DIR}"

# ---------- 1. 基础依赖 ----------
if [[ "$SKIP_BUILD" == "1" ]]; then
  if ! command -v curl >/dev/null 2>&1; then
    if command -v apt-get >/dev/null 2>&1; then apt-get update -qq && apt-get install -y -qq curl >/dev/null
    elif command -v yum >/dev/null 2>&1; then yum install -y -q curl >/dev/null; fi
  fi
else
info "安装基础依赖（git / curl / rsync）..."
if command -v apt-get >/dev/null 2>&1; then
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq
  apt-get install -y -qq git curl rsync ca-certificates >/dev/null
elif command -v yum >/dev/null 2>&1; then
  yum install -y -q git curl rsync ca-certificates >/dev/null
else
  fail "未识别的包管理器（apt/yum），请手动安装 git、curl、rsync 后重试"
fi
fi

# ---------- 2. Go 工具链（不存在则下载官方发行版） ----------
install_go() {
  local dst="/usr/local/go"
  if [[ -x "$dst/bin/go" ]]; then
    local have; have="$("$dst/bin/go" version | awk '{print $3}' | sed 's/^go//')"
    info "检测到 Go $have，跳过安装（如需升级设 GO_VERSION）"
    return
  fi
  local dl_base="${GO_DL_BASE:-https://go.dev/dl}"
  info "下载 Go ${GO_VERSION} (linux-${GO_ARCH}) ← ${dl_base} ..."
  local tarball="go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
  curl -fsSL "${dl_base}/${tarball}" -o "/tmp/${tarball}" || fail "Go 下载失败（可设 GO_DL_BASE 换镜像，如 https://mirrors.aliyun.com/golang）"
  rm -rf "$dst"
  tar -C /usr/local -xzf "/tmp/${tarball}"
  rm -f "/tmp/${tarball}"
}
if [[ "$SKIP_BUILD" == "1" ]]; then
  :
else
install_go
export PATH="/usr/local/go/bin:$PATH"
export GOTOOLCHAIN=local
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"   # 国内镜像加速依赖
go version
fi

# ---------- 3. 获取代码并编译 ----------
if [[ "$SKIP_BUILD" == "1" ]]; then
  info "使用预编译二进制: ${APP_BIN}"
  cp "$APP_BIN" /tmp/zhihudp.bin
else
  BUILD_DIR="$(mktemp -d /tmp/zhihudp-build.XXXXXX)"
  trap 'rm -rf "$BUILD_DIR"' EXIT
  if [[ -d "$APP_SOURCE" ]]; then
    info "从本地目录上传代码: $APP_SOURCE"
    rsync -a --exclude '.git' --exclude 'config.yaml' --exclude '*.pem' "$APP_SOURCE/" "$BUILD_DIR/"
  else
    info "克隆代码: $APP_SOURCE（分支: ${GIT_BRANCH:-默认}）"
    if [[ -n "$GIT_BRANCH" ]]; then
      git clone --depth 1 -q -b "$GIT_BRANCH" "$APP_SOURCE" "$BUILD_DIR" \
        || fail "git clone 失败：仓库是否公开？私有仓库请把代码 rsync 到服务器后设 APP_SOURCE=本地目录"
    else
      git clone --depth 1 -q "$APP_SOURCE" "$BUILD_DIR" \
        || fail "git clone 失败：仓库是否公开？私有仓库请把代码 rsync 到服务器后设 APP_SOURCE=本地目录"
    fi
  fi
  info "编译（go:embed 内嵌前端，产物为单文件）..."
  ( cd "$BUILD_DIR" && go build -trimpath -ldflags "-s -w" -o /tmp/zhihudp.bin ./cmd/server ) \
    || fail "编译失败（Go 版本需 ≥ go.mod 要求；可设 GO_VERSION 升级）"
fi

# ---------- 4. 运行用户与目录 ----------
info "创建运行用户 ${APP_USER}（非 root）..."
if ! id "$APP_USER" >/dev/null 2>&1; then
  useradd -r -m -d "$APP_DIR" -s /usr/sbin/nologin "$APP_USER"
fi
mkdir -p "$APP_DIR/.zhihudp"
install -o "$APP_USER" -g "$APP_USER" -m 0755 /tmp/zhihudp.bin "$APP_DIR/zhihudp"
rm -f /tmp/zhihudp.bin
chown -R "$APP_USER":"$APP_USER" "$APP_DIR"

# ---------- 5. 密钥配置（密文入库，私钥仅本机） ----------
# 5.0 收集密钥：环境变量 > 交互输入 > 留空（开屏上传）
DEEPSEEK_API_KEY="${DEEPSEEK_API_KEY:-}"
if [[ -z "$DEEPSEEK_API_KEY" ]]; then
  warn "未检测到环境变量密钥（DEEPSEEK_API_KEY）"
  read -r -p "是否现在交互输入密钥？[y/N] " _ans
  if [[ "$_ans" =~ ^[Yy]$ ]]; then
    read -r -s -p "DeepSeek API Key: " DEEPSEEK_API_KEY; echo
  else
    warn "跳过：部署后打开网页，在开屏弹窗里上传密钥（同样 RSA 加密持久化）"
  fi
fi

# 知乎 Access Secret（emotion 分支情绪分析必需；main 分支无此配置忽略即可）
ZHIHU_ACCESS_SECRET="${ZHIHU_ACCESS_SECRET:-}"
if [[ -n "$ZHIHU_ACCESS_SECRET" ]]; then
  info "检测到 ZHIHU_ACCESS_SECRET，将加密写入 config.yaml 的 zhihu 段"
fi

# 5.1 生成持久 RSA 密钥对（若私钥不存在）
if [[ ! -f "$PRIVATE_KEY" ]]; then
  info "生成持久 RSA 密钥对（私钥: ${PRIVATE_KEY}）..."
  cat > "$CONFIG_PATH" <<EOF
keybox:
  private_key: "${PRIVATE_KEY}"
server:
  port: ${APP_PORT}
EOF
  chown "$APP_USER":"$APP_USER" "$CONFIG_PATH"
  su -s /bin/bash "$APP_USER" -c "/opt/zhihudp/zhihudp -keygen -config ${CONFIG_PATH}" </dev/null >/dev/null 2>&1 || true
  [[ -f "$PRIVATE_KEY" ]] || fail "密钥对生成失败（$PRIVATE_KEY 不存在）"
  chmod 600 "$PRIVATE_KEY"
  info "私钥已生成（chmod 600）。请勿外传——它是唯一能解密密文的钥匙"
else
  info "复用已有私钥 ${PRIVATE_KEY}"
fi

# 5.1.5 媒体播放令牌（随机生成；也可设 MEDIA_TOKEN 指定）
MEDIA_TOKEN="${MEDIA_TOKEN:-$(head -c 16 /dev/urandom | od -An -tx1 | tr -d ' \n')}"

# 5.2 用公钥加密真实密钥 → 密文写入 config.yaml（明文只在脚本内存，不落盘）
# 注意：私钥属主是 APP_USER，故以该用户运行 -enc 读取私钥
encrypt_key() {
  local q; q="$(printf '%q' "$1")"
  su -s /bin/bash "$APP_USER" -c "/opt/zhihudp/zhihudp -enc ${q} -config ${CONFIG_PATH}" 2>/dev/null || true
}
DEEPSEEK_ENC=""
if [[ -n "$DEEPSEEK_API_KEY" ]]; then DEEPSEEK_ENC="$(encrypt_key "$DEEPSEEK_API_KEY")"; fi
ZHIHU_ENC=""
if [[ -n "$ZHIHU_ACCESS_SECRET" ]]; then ZHIHU_ENC="$(encrypt_key "$ZHIHU_ACCESS_SECRET")"; fi

# 5.2.5 升级场景：环境变量未提供密钥时，保留旧 config.yaml 中已有密文（不覆盖不丢密钥）
if [[ -f "$CONFIG_PATH" ]]; then
  OLD_DEEPSEEK_ENC="$(awk '/api_key_enc:/{gsub(/[",]/, "", $2); print $2; exit}' "$CONFIG_PATH" 2>/dev/null || true)"
  OLD_ZHIHU_ENC="$(awk '/access_secret_enc:/{gsub(/[",]/, "", $2); print $2; exit}' "$CONFIG_PATH" 2>/dev/null || true)"
  [[ -z "$DEEPSEEK_ENC" && -n "$OLD_DEEPSEEK_ENC" ]] && { DEEPSEEK_ENC="$OLD_DEEPSEEK_ENC"; info "复用已有 DeepSeek 密文"; }
  [[ -z "$ZHIHU_ENC" && -n "$OLD_ZHIHU_ENC" ]] && { ZHIHU_ENC="$OLD_ZHIHU_ENC"; info "复用已有知乎密文"; }
fi

# 5.3 组装最终 config.yaml（仅密文）
{
  echo "keybox:"
  echo "  private_key: \"${PRIVATE_KEY}\""
  if [[ -n "$ZHIHU_ENC" ]]; then
    echo "zhihu:"
    echo "  access_secret_enc: \"${ZHIHU_ENC}\""
    echo "  openapi_base_url: \"https://developer.zhihu.com\""
  fi
  echo "deepseek:"
  echo "  api_key_enc: \"${DEEPSEEK_ENC}\""
  echo "  base_url: \"https://api.deepseek.com\""
  echo "  timeout: 120s"
  echo "media:"
  echo "  dir: \"/opt/zhihudp/media\""
  echo "  token: \"${MEDIA_TOKEN}\""
  echo "server:"
  echo "  port: ${APP_PORT}"
} > "$CONFIG_PATH"
chown "$APP_USER":"$APP_USER" "$CONFIG_PATH"
chmod 600 "$CONFIG_PATH"
mkdir -p /opt/zhihudp/media
chown "$APP_USER":"$APP_USER" /opt/zhihudp/media
info "config.yaml 已生成（仅密文，无明文；chmod 600）"

# ---------- 6. systemd 服务 ----------
info "安装 systemd 服务 zhihudp.service..."
cat > /etc/systemd/system/zhihudp.service <<EOF
[Unit]
Description=zhihuDP - Zhihu Stock Sentiment Analysis
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${APP_USER}
Group=${APP_USER}
WorkingDirectory=${APP_DIR}
ExecStart=${APP_DIR}/zhihudp -config ${CONFIG_PATH}
Restart=on-failure
RestartSec=3
Environment=GOTOOLCHAIN=local
# 安全加固
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${APP_DIR}
EOF
systemctl daemon-reload
systemctl enable zhihudp.service >/dev/null 2>&1
systemctl restart zhihudp.service
sleep 2

# ---------- 7. 健康检查 ----------
info "健康检查..."
if systemctl is-active --quiet zhihudp.service; then
  curl -fsS -o /dev/null "http://127.0.0.1:${APP_PORT}/" \
    && info "✅ 部署成功：http://<服务器公网IP>:${APP_PORT}（首页 200）" \
    || warn "服务已启动但首页探测失败，请查看日志：journalctl -u zhihudp -n 50"
else
  fail "服务启动失败，日志：journalctl -u zhihudp -n 50"
fi

# ---------- 8. 防火墙 ----------
if command -v ufw >/dev/null 2>&1; then
  ufw allow "${APP_PORT}/tcp" >/dev/null 2>&1 && info "ufw 已放行 ${APP_PORT}/tcp" || warn "ufw 放行失败，请手动开放 ${APP_PORT}"
elif command -v firewall-cmd >/dev/null 2>&1; then
  firewall-cmd --permanent --add-port="${APP_PORT}/tcp" >/dev/null 2>&1 && firewall-cmd --reload >/dev/null 2>&1 && info "firewalld 已放行 ${APP_PORT}/tcp"
fi

cat <<EOF

════════════════════════════════════════════════════════════════
 部署完成 ✔
   服务    : systemctl status zhihudp
   日志    : journalctl -u zhihudp -f
   访问    : http://<公网IP>:${APP_PORT}
   密钥    : 私钥 ${PRIVATE_KEY}（勿外传）
             已配置 → 开屏弹窗可再上传（加密持久化）
   配额    : 每次打开页面 20 次 API 调用（防滥用）
   升级    : 重新执行本脚本即可（复用私钥，热更二进制）
 注意     : 云安全组需放行 TCP ${APP_PORT}；对外建议用 Caddy/nginx
             做 HTTPS 反向代理，避免明文流量
════════════════════════════════════════════════════════════════
EOF
