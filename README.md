# 知乎大盘 · 知乎股票情绪分析工具

> 把知乎上关于一只股票的嘈杂讨论，整理成一眼看懂的结构化情绪信号。**信息工具，不构成投资建议。**

<div align="center">

**知乎搜索 · AI 情绪分类 · 参考强度分 · 流式分析 · 看山追问对话**

[GitHub](https://github.com/ZZ2416/zhihuDP) · [联系与反馈](https://github.com/ZZ2416/zhihuDP/issues)

</div>

---

## 功能特性

- 🔍 **股票识别**：输入名称或代码（`茅台` / `600519`），东财主选 + 腾讯兜底
- 📊 **情绪面板**：讨论热度、多空占比（看多/看空/中性）、参考强度分（1-10）、代表观点（带原帖链接）
- 📈 **行情**：实时报价 + **日K线**（MA5/10/20、成交量、十字光标交互）+ **当日分时图**（日K/分时切换，价格线/均价线/昨收参考线，**可交互 + 30s 实时刷新**）
- 📑 **财务解析**：东财双源抓取核心财务指标（最新报告期 + 5 年年报），**AI 流式解读**盈利/成长/财务健康/风险（只解读不荐股）
- 🤖 **AI 分析**：基于真实知乎讨论流式生成三段解读（情绪面总结 / 值得关注的讨论点 / 风险提示）
- 🦊 **与看山对话（二期）**：分析完成后可就结果**多轮追问**——「AI 看山」人设（刘看山）结合行情快照、**财报指标**、情绪面板、知识库与前次分析回答，只给方法/维度/风险提示，不荐股
- 🔥 **热门板块 / 暴跌板块 / 热门股票**：侧边栏实时榜单，涨幅**四档分级配色**（跌绿 / 0~2% 黄 / 2~4% 橙 / ≥4% 红），点击直达查询
- 📚 **讨论文章**：知乎知识库方形卡片，链接可用性校验（失效不展示）
- 📰 **相关资讯**：东方财富资讯，标题可点击跳转原文
- 🎬 **相关视频**：B站视频资讯（wbi 签名搜索），**封面卡片横向滑动**（5 条），**按最近时间 / 播放量排序**切换，点击跳转
- 🔑 **密钥配置弹窗**：开屏动画 + 首次进入可配置自己的 DeepSeek / 知乎密钥（可跳过用默认）；用户密钥 **RSA 公私钥加密**传输，默认密钥绝不下发明文
- ⚡ **SSE 流式输出**：分析面板逐字呈现
- 🌙 **明暗主题**：手动切换 + 跟随系统偏好
- 🛡️ **合规设计**：不输出买卖结论 / 不用「概率」措辞 / 输出禁用词过滤 / 全程免责声明
- 📝 **友好降级**：股票不存在 / 数据不足 / 搜索失败时优雅提示，不报技术错误

## 界面截图（按时间排序）

<img src="pic/Snipaste_2026-08-17_00-14-49.png" width="640" alt="截图 1">

<img src="pic/Snipaste_2026-08-17_00-15-17.png" width="640" alt="截图 2">

<img src="pic/Snipaste_2026-08-17_00-15-39.png" width="640" alt="截图 3">

<img src="pic/Snipaste_2026-08-17_00-15-54.png" width="640" alt="截图 4">

## 版本变更

| 版本 | 里程碑 | 说明 |
|---|---|---|
| **v2.3** | 分时图 + B站视频 | 分时图交互与 30s 实时刷新、查询中占位；B站视频资讯（wbi 签名）封面卡片横滑、按时间/播放量排序 |
| v2.2 | 财报解析 + 分时图 | 东财双源财务指标（5年年报）+ AI 流式解读；日K/分时切换；看山对话注入财报上下文；东财资讯可跳转原文 |
| v2.0 | 与看山对话 | 二期：多轮追问对话（AI 看山 Agent，按股票隔离会话）、行情快照/知识库/前次分析注入上下文、合规过滤强化 |
| v1.x | 一期核心 | 个股识别 → 行情/K线 → 情绪面板 → AI 分析；热门榜单、讨论文章、密钥弹窗（RSA 加密）、看山主题 |

> 完整逐条变更见 [docs/CHANGELOG.md](docs/CHANGELOG.md)。

## 技术栈

| 层 | 选型 |
|---|---|
| 语言 | Go 1.25 |
| Agent 框架 | [cloudwego/eino](https://github.com/cloudwego/eino) v0.9.14（ADK ChatModelAgent，ReAct 循环） |
| 模型 | DeepSeek `deepseek-chat`（eino-ext 组件 v0.1.7） |
| 数据源 | 知乎开放平台 `zhihu_search`（Bearer 鉴权） |
| Web | Go 标准库 `net/http` + SSE，前端原生 HTML/JS（零构建，go:embed 内嵌） |

## 快速开始

### 1. 获取密钥（2 个）

| 密钥 | 获取地址 |
|---|---|
| 知乎 Access Secret | https://developer.zhihu.com/profile |
| DeepSeek API Key | https://platform.deepseek.com → API Keys |

### 2. 配置（配置文件模式，密钥不入库）

```bash
cd zhihuDP
cp config.example.yaml config.yaml   # 填入你的密钥
```

```yaml
zhihu:
  access_secret: "你的知乎AccessSecret"   # 必填
deepseek:
  api_key: "sk-你的DeepSeekKey"          # 必填
```

> `config.yaml` 已被 `.gitignore` 排除，不会进入版本库；权限建议 `chmod 600 config.yaml`。

### 3. 启动

```bash
go run ./cmd/server
# 浏览器访问 http://localhost:8080
```

### 4. CLI 快速验证（可选）

```bash
go run ./cmd/server -q 茅台    # 跑一次完整分析并打印事件，不启动 HTTP
```

## API

### POST /api/ask（SSE 流式）

```bash
curl -N -X POST http://localhost:8080/api/ask \
  -H "Content-Type: application/json" \
  -d '{"stock":"茅台"}'
```

事件协议：`stock`（股票识别）→ `sentiment`（情绪面板 JSON）→ `delta`×N（流式分析）→ `done` / `error`

### POST /api/chat（二期 · 看山追问对话，SSE 流式）

```bash
curl -N -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d '{"stock":"600519","market":"沪A","message":"情绪为什么偏空？"}'
```

- 会话按股票代码隔离（多轮上下文）；`POST /api/chat/reset` 清空会话
- 事件协议同 `/api/ask`（`delta` → `done` / `error`）

### GET /api/resolve?q=（探针）

```bash
curl "http://localhost:8080/api/resolve?q=600519"
# {"code":"600519","name":"贵州茅台","market":"沪A"}
```

### 其他接口

| 接口 | 说明 |
|---|---|
| `GET /api/kline?code=&market=&days=` | 行情：报价 + 日K线 |
| `GET /api/minute?code=&market=` | 当日分时数据（东财主 + 腾讯兜底） |
| `GET /api/finance?code=&market=` | 财务指标（5年年报 + 最新期，东财双源） |
| `POST /api/finance/analyze` | 财报 AI 解析（SSE 流式，计配额） |
| `GET /api/hot?type=stock\|sector\|sector_fall\|sector_stock` | 热门股票 / 上升板块 / 暴跌板块 / 板块成分股 |
| `GET /api/news?keyword=&count=` | 相关资讯（标题可跳转原文） |
| `GET /api/video?keyword=&count=` | 相关视频（B站，前端按时间/播放量排序） |
| `GET /api/knowledge?q=&limit=` | 知识库搜索（讨论文章卡片，失效链接已过滤） |
| `GET /api/config/pubkey` | 下发 RSA 公钥（密钥弹窗加密用） |
| `POST /api/config/keys` | 提交加密后的用户密钥（RSA-OAEP，私钥仅服务端） |

## 项目结构（标准 cmd/internal + Router/Handler/Service/Data 四层）

```
zhihuDP/
├── cmd/server/            # 入口：配置加载 → 依赖组装 → 启动（含 CLI 模式）
├── internal/
│   ├── server/            # HTTP 层：Router + Handler（依赖 Analyzer/FinanceProvider 等接口）
│   ├── agent/             # controller 层：eino ADK ReAct 编排 + 看山对话 + 财报解析
│   ├── chat/              # service 层（二期）：会话存储（按股票隔离）+ 上下文事实组装
│   ├── sentiment/         # service 层：情绪分析（LLM 分类 + 规则强度分）
│   ├── finance/           # dao 层：财务指标（东财 datacenter 主 + F10 兜底）
│   ├── minute/            # dao 层：当日分时（东财 trends2 主 + 腾讯兜底）
│   ├── video/             # dao 层：B站视频搜索（wbi 签名）
│   ├── zhihu/             # dao 层：知乎搜索 + 知识库 RAG 客户端（Bearer 鉴权）
│   ├── stock/             # dao 层：股票识别（东财 + 腾讯）
│   ├── kline/             # dao 层：行情 K线（腾讯，除权处理）
│   ├── hot/               # dao 层：热门股票 / 板块榜（腾讯，涨幅/跌幅本地排序）
│   ├── news/              # dao 层：相关资讯（东财，可跳转原文）
│   ├── keybox/            # 密钥箱：RSA 公私钥（弹窗密钥加密传输 + 持久密文）
│   ├── compliance/        # 合规禁用词过滤
│   ├── config/            # 配置文件加载（YAML + env 覆盖 + 脱敏）
│   ├── types/             # 共享结构 + 哨兵错误
│   └── web/               # 前端（独立分层：index.html + css/ + js/ 模块）
└── docs/CHANGELOG.md      # 变更记录
```

## 文档链

| 文档 | 内容 |
|---|---|
| [business-design.md](business-design.md) | 业务定位、规则语义、合规红线、8 项验收标准 |
| [product-design.md](product-design.md) | 产品设计稿（线框、交互、文案规范） |
| [tech-design.md](tech-design.md) | 技术设计（架构、时序、依赖锁定、API 协议） |
| [features/v2/SRS.md](features/v2/SRS.md) | 二期需求规格（看山对话，v0.2 定稿） |
| [features/v2/SDD.md](features/v2/SDD.md) | 二期技术设计（会话/快照/人设/合规） |
| [features/finance/设计方案.md](features/finance/设计方案.md) | 财报解析设计方案（东财双源 + AI 解读） |
| [features/finance/SRS.md](features/finance/SRS.md) | 财报解析需求规格 |
| [features/finance/SDD.md](features/finance/SDD.md) | 财报解析技术设计 |
| [features/minute/SRS.md](features/minute/SRS.md) | 分时图需求规格 |
| [features/minute/SDD.md](features/minute/SDD.md) | 分时图技术设计 |
| [features/video/SRS.md](features/video/SRS.md) | B站相关视频需求规格 |
| [features/video/SDD.md](features/video/SDD.md) | B站相关视频技术设计 |
| [docs/CHANGELOG.md](docs/CHANGELOG.md) | 变更记录 |

## 路线图

- [x] **阶段 1（一期）**：个股识别 → 行情/K线 → 情绪面板 → AI 分析（流式）
- [x] **阶段 2（二期）**：与看山对话（多轮追问 Agent）、热门/暴跌板块、讨论文章、密钥安全
- [ ] 阶段 3：情绪分类正式评测基准、历史会话回看、真实用户试用
- [ ] 阶段 4：合规审查、变现模式、日频快照

## 免责声明

> 本工具仅整理展示知乎公开讨论信息，不构成任何投资建议，不构成对任何证券的买入、卖出或持有建议。据此操作，风险自担。参考强度反映讨论情绪与数据的充分程度，不代表涨跌预测。数据来源于知乎公开讨论，与知乎官方无关。

## 许可证

暂未指定（个人项目）。
