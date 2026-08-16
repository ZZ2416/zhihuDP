# 知乎大盘 · 知乎股票情绪分析工具

> 把知乎上关于一只股票的嘈杂讨论，整理成一眼看懂的结构化情绪信号。**信息工具，不构成投资建议。**

<div align="center">

**知乎搜索 · AI 情绪分类 · 参考强度分 · 流式分析**

[GitHub](https://github.com/ZZ2416/zhihuDP) · [联系与反馈](https://github.com/ZZ2416/zhihuDP/issues)

</div>

---

## 功能特性

- 🔍 **股票识别**：输入名称或代码（`茅台` / `600519`），东财主选 + 腾讯兜底
- 📊 **情绪面板**：讨论热度、多空占比（看多/看空/中性）、参考强度分（1-10）、代表观点（带原帖链接）
- 🤖 **AI 分析**：基于真实知乎讨论流式生成三段解读（情绪面总结 / 值得关注的讨论点 / 风险提示）
- ⚡ **SSE 流式输出**：分析面板逐字呈现
- 🌙 **明暗主题**：手动切换 + 跟随系统偏好
- 🐋 **知乎看山风格**：知乎蓝设计系统 + 鲸鱼元素
- 🛡️ **合规设计**：不输出买卖结论 / 不用「概率」措辞 / 输出禁用词过滤 / 全程免责声明
- 📝 **友好降级**：股票不存在 / 数据不足 / 搜索失败时优雅提示，不报技术错误

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

### GET /api/resolve?q=（探针）

```bash
curl "http://localhost:8080/api/resolve?q=600519"
# {"code":"600519","name":"贵州茅台","market":"沪A"}
```

## 项目结构（标准 cmd/internal + Router/Handler/Service/Data 四层）

```
zhihuDP/
├── cmd/server/            # 入口：配置加载 → 依赖组装 → 启动（含 CLI 模式）
├── internal/
│   ├── server/            # HTTP 层：Router + Handler（依赖 Analyzer/Resolver 接口）
│   ├── agent/             # controller 层：eino ADK ReAct 编排 + 事件分发
│   ├── sentiment/         # service 层：情绪分析（LLM 分类 + 规则强度分）
│   ├── zhihu/             # dao 层：知乎搜索客户端（Bearer 鉴权）
│   ├── stock/             # dao 层：股票识别（东财 + 腾讯）
│   ├── compliance/        # 合规禁用词过滤
│   ├── config/            # 配置文件加载（YAML + env 覆盖 + 脱敏）
│   ├── types/             # 共享结构 + 哨兵错误
│   └── web/               # 前端（独立分层：index.html + css/ + js/ 八模块）
└── docs/CHANGELOG.md      # 变更记录
```

## 文档链

| 文档 | 内容 |
|---|---|
| [business-design.md](business-design.md) | 业务定位、规则语义、合规红线、8 项验收标准 |
| [product-design.md](product-design.md) | 产品设计稿（线框、交互、文案规范） |
| [tech-design.md](tech-design.md) | 技术设计（架构、时序、依赖锁定、API 协议） |
| [docs/CHANGELOG.md](docs/CHANGELOG.md) | 变更记录 |

## 路线图

- [ ] 阶段 2：行情数据接入（知乎接口）、情绪分类正式评测基准、真实用户试用
- [ ] 阶段 3：合规审查、变现模式、日频快照

## 免责声明

> 本工具仅整理展示知乎公开讨论信息，不构成任何投资建议，不构成对任何证券的买入、卖出或持有建议。据此操作，风险自担。参考强度反映讨论情绪与数据的充分程度，不代表涨跌预测。数据来源于知乎公开讨论，与知乎官方无关。

## 许可证

暂未指定（个人项目）。
