#!/usr/bin/env bash
# =============================================================================
# zhihuDP HTTPS 一键配置（nginx 自签证书反向代理）
# -----------------------------------------------------------------------------
# 背景：公网 HTTP（如 http://43.157.57.53:8080）下浏览器视为「非安全上下文」，
#       Web Crypto（crypto.subtle）不可用 → 密钥弹窗报
#       "Cannot read properties of undefined (reading 'importKey')"。
# 解决：本脚本装 nginx + 生成自签证书（含公网 IP SAN）→ HTTPS 反向代理到
#       127.0.0.1:8080 → 访问 https://<IP>:8443（首次点「继续前往」），
#       页面即视为安全上下文，密钥弹窗恢复正常。
#
# 用法（服务器上 root 执行）：
#   bash https-setup.sh [公网IP]      # 不传则自动获取
# 可选环境变量：HTTPS_PORT=8443  APP_PORT=8080
# =============================================================================
set -euo pipefail

SERVER_IP="${1:-$(curl -s --max-time 5 ifconfig.me)}"
HTTPS_PORT="${HTTPS_PORT:-8443}"
APP_PORT="${APP_PORT:-8080}"

C_GRN='\033[0;32m'; C_YLW='\033[1;33m'; C_NC='\033[0m'
info() { echo -e "${C_GRN}[https]${C_NC} $*"; }
warn() { echo -e "${C_YLW}[https]${C_NC} $*"; }

[[ $EUID -eq 0 ]] || { echo "[https] 请用 root 执行"; exit 1; }
[[ -n "$SERVER_IP" ]] || SERVER_IP="127.0.0.1"
info "公网 IP: ${SERVER_IP}，HTTPS 端口: ${HTTPS_PORT} → 反代 127.0.0.1:${APP_PORT}"

# ---------- 1. 安装 nginx + openssl ----------
info "安装 nginx / openssl ..."
if command -v apt-get >/dev/null 2>&1; then
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq
  apt-get install -y -qq nginx openssl >/dev/null
elif command -v yum >/dev/null 2>&1; then
  yum install -y -q nginx openssl >/dev/null
else
  echo "[https] 未识别的包管理器"; exit 1
fi

# ---------- 2. 自签证书（SAN 含公网 IP / localhost / 127.0.0.1，10 年） ----------
info "生成自签证书（SAN: ${SERVER_IP}, localhost, 127.0.0.1）..."
mkdir -p /etc/nginx/ssl
openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout /etc/nginx/ssl/zhihudp-key.pem \
  -out    /etc/nginx/ssl/zhihudp-cert.pem \
  -days 3650 -subj "/CN=zhihudp" \
  -addext "subjectAltName=IP:${SERVER_IP},DNS:localhost,IP:127.0.0.1" \
  >/dev/null 2>&1 || { echo "[https] 证书生成失败"; exit 1; }
chmod 600 /etc/nginx/ssl/zhihudp-key.pem

# ---------- 3. nginx 站点：HTTPS 反代（SSE 关缓冲） ----------
info "写入 nginx 站点配置（https://:${HTTPS_PORT} → 127.0.0.1:${APP_PORT}）..."
cat > /etc/nginx/sites-available/zhihudp-https <<EOF
server {
    listen ${HTTPS_PORT} ssl;
    http2 on;
    server_name _;

    ssl_certificate     /etc/nginx/ssl/zhihudp-cert.pem;
    ssl_certificate_key /etc/nginx/ssl/zhihudp-key.pem;

    client_max_body_size 200m;

    location / {
        proxy_pass http://127.0.0.1:${APP_PORT};
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
        proxy_buffering off;   # SSE 流式输出必需
        proxy_cache off;
        proxy_read_timeout 300s;
        proxy_send_timeout 300s;
    }
}
EOF
ln -sf /etc/nginx/sites-available/zhihudp-https /etc/nginx/sites-enabled/zhihudp-https
rm -f /etc/nginx/sites-enabled/default

nginx -t || { echo "[https] nginx 配置校验失败"; exit 1; }
systemctl enable nginx >/dev/null 2>&1 || true
systemctl reload nginx || systemctl restart nginx

# ---------- 4. 防火墙 ----------
if command -v ufw >/dev/null 2>&1; then
  ufw allow "${HTTPS_PORT}/tcp" >/dev/null 2>&1 && info "ufw 已放行 ${HTTPS_PORT}/tcp" || warn "ufw 放行失败，请手动开放 ${HTTPS_PORT}"
fi

cat <<EOF

════════════════════════════════════════════════════════════════
 HTTPS 配置完成 ✔
   访问    : https://${SERVER_IP}:${HTTPS_PORT}
   证书    : /etc/nginx/ssl/zhihudp-cert.pem（自签，10 年）
   说明    : 首次访问浏览器会警告「证书不受信任」，
             点「高级 → 继续前往」即可（页面即被视为安全上下文，
             密钥弹窗恢复正常，不再报 importKey 错误）

 可选（彻底消除警告，推荐）：
   1. 在服务器上执行：
        base64 /etc/nginx/ssl/zhihudp-cert.pem
   2. 把输出的 base64 保存为 zhihudp.crt，传到本机
   3. 本机安装信任：
        macOS: 双击证书 → 钥匙串 → 始终信任
        Windows: 双击 → 安装证书 → 本地计算机 → 受信任的根证书颁发机构
   之后访问 https://${SERVER_IP}:${HTTPS_PORT} 无警告

 别忘了：腾讯云控制台安全组需放行 TCP ${HTTPS_PORT}
════════════════════════════════════════════════════════════════
EOF
