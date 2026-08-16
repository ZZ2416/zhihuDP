# 云服务器一键部署（zhihuDP）

`deploy/install.sh` 在**空云服务器**（全新 Ubuntu / Debian / CentOS，amd64 / arm64）上完成全部部署：
装 Go → 拉代码 → 编译（单文件，前端已内嵌）→ 生成 RSA 密钥对 → 密钥密文入库 → systemd 服务 → 开机自启 → 健康检查 → 防火墙放行。

## 快速开始

### 方式一：服务器完整编译（首次 / 服务器网络好时）

```bash
# 上传脚本（本地执行）
scp deploy/install.sh root@<服务器IP>:/

# 在服务器上执行（推荐方式 A：环境变量传密钥，明文不落盘）
ssh root@<服务器IP>
DEEPSEEK_API_KEY="sk-xxx" ZHIHU_ACCESS_SECRET="xxx" bash /install.sh
```

### 方式二：本地交叉编译 + 上传（上传慢 / 服务器装 Go 慢时的最快路径）⭐

```bash
# 本地执行：交叉编译（Linux amd64 单文件 21M）→ scp 上传 → 远程快速部署
./deploy/upload.sh root@<服务器IP>

# 带密钥（明文不落盘到服务器）
DEEPSEEK_API_KEY="sk-xxx" ZHIHU_ACCESS_SECRET="xxx" ./deploy/upload.sh root@<服务器IP>

# arm64 服务器
ARCH=arm64 ./deploy/upload.sh root@<服务器IP>
```

> 原理：服务器检测到预编译二进制（`APP_BIN`）后**跳过装 Go / 拉代码 / 编译**，
> 只做 用户/密钥/systemd/健康检查，秒级完成。之后每次升级只需重跑 upload.sh（复用私钥与密文）。

## 上传慢的其他加速手段

| 卡点 | 解法 |
|---|---|
| 服务器下载 Go 工具链慢 | `GO_DL_BASE=https://mirrors.aliyun.com/golang bash install.sh` |
| 服务器拉取 Go 依赖慢 | 已内置 `GOPROXY=https://goproxy.cn,direct`（国内镜像） |
| git clone GitHub 慢 | 方式二直接传二进制，绕开 clone；或本地 `rsync` 代码后设 `APP_SOURCE=本地目录` |
| scp 本身慢 | 先压缩再传：`xz /tmp/zhihudp.bin`（21M→约8M），服务器 `xz -d` 后执行 |

部署完成后浏览器访问 `http://<公网IP>:8080`。

## 三种密钥提供方式

| 方式 | 命令 | 说明 |
|---|---|---|
| A. 环境变量（推荐） | `DEEPSEEK_API_KEY=... ZHIHU_ACCESS_SECRET=... bash install.sh` | 明文只在脚本内存，服务器 config.yaml 只存密文 |
| B. 交互输入 | `bash install.sh` → 按提示输入 | 终端不回显 |
| C. 部署后开屏上传 | `bash install.sh`（跳过密钥） | 打开页面后在开屏弹窗上传，同样 RSA 加密持久化 |

## 常用参数

| 环境变量 | 默认 | 说明 |
|---|---|---|
| `APP_SOURCE` | `https://github.com/ZZ2416/zhihuDP.git` | 代码来源；**私有仓库**请先 `rsync` 代码到服务器，再设为本机目录 |
| `GO_VERSION` | `1.25.5` | Go 版本 |
| `APP_PORT` | `8080` | 监听端口（systemd + 防火墙同步） |
| `APP_USER` | `zhihudp` | 运行用户（非 root） |
| `APP_DIR` | `/opt/zhihudp` | 安装目录 |

## 日常运维

```bash
systemctl status zhihudp        # 状态
journalctl -u zhihudp -f        # 日志
systemctl restart zhihudp       # 重启
bash install.sh                 # 升级（重新执行：拉新代码编译，复用私钥与密文）
```

## 密钥与安全（务必读）

- **私钥**：`/opt/zhihudp/.zhihudp/zhihudp_private.pem`（chmod 600）——**唯一能解密密文的钥匙，勿外传**；`config.yaml` 只有密文，即使仓库/配置泄露也解不开
- **配额**：内置「每次打开页面 20 次 API 调用」防滥用（新浏览器/清 Cookie 重新 20 次）
- **公网建议**：
  1. 云安全组放行 `APP_PORT`（脚本已配 ufw/firewalld）
  2. 用 Caddy / nginx 做 **HTTPS 反向代理**（脚本直接监听 0.0.0.0，明文 HTTP 不适合公网生产）
  3. 私钥在服务器上 = 服务器管理员可读；如需更强隔离，可让服务器与密钥分离部署（进阶，暂未支持）
