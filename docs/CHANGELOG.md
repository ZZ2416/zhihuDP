# 变更记录（CHANGELOG）

> 项目：zhihuDP — 知乎股票情绪分析工具
> 起始日期：2026-08-16
> 说明：与 git 提交一一对应；文档链与代码均记录于此。

---

## 2026-08-16 · ui_premiu：两个待确认项落地

**① 板块点击 → 成分股**：
- `internal/hot` 新增 `GetSectorStocks`（腾讯 board_code=板块代码）
- `/api/hot?type=sector_stock&code=pt01801712&count=`（HotProvider 接口扩展）
- 前端：板块 chip 可点击 → 热门股票卡片切换为「板块成分 · XX」+ 「← 返回热门」

**② ticker 自动滚动**：
- CSS 无缝循环动画（双份内容 + `translateX(-50%)`）+ 悬停暂停 + `prefers-reduced-motion` 适配

**数据源切换（实测驱动）**：东财 push2 再次被限流（全镜像 000）→ 热门数据改**腾讯源**（与 K线同源、稳定）：
- 热门股票 = 成交额榜（腾讯无涨幅排序；成交额=市场活跃度，比涨幅榜更符合「热门」语义）
- 热门板块 = `mktHs/rank?t=01/averatio`（板块涨幅榜）
- 成分股 = `getBoardRankList?board_code={pt码}`（腾讯数值字段为字符串，已适配）

**验证**：股票 6 条（兆易创新/亨通光电...）✅、板块（玻璃玻纤 4.31%）✅、成分股（中国巨石...）✅、lint/test 全绿 ✅

---

## 2026-08-16 · ui_premiu：首页热门推荐 + 看山吉祥物主题

**需求/设计**：`features/ui_premiu/SRS.md` + `SDD.md`

**新增**：
- `internal/hot`（dao 层）：东财涨幅榜（热门股票/行业板块，免登录）
- `GET /api/hot?type=stock|sector&count=` + `HotProvider` 接口
- 首页重构：**热门板块 chips**（6 个）+ **热门股票列表**（8 只，点击直达查询）+ **ticker 终端行情条**（可点击）
- **看山吉祥物**：`pic/` 素材 → `web/pic/kanshan.png`（go:embed），首页 hero 展示
- 终端风格交互（对标「看山指数」页）：ticker 滚动条 + 热门面板，**保留双主题与全部功能**
- 热门数据为展示数据，不进 LLM（合规）

**验证**：/api/hot 股票 5 条 + 板块 4 条 ✅、参数校验 400 ✅、首页新元素 4 处 ✅、素材 200 ✅、lint/test/JS 语法全绿 ✅

---

## 2026-08-16 · feature/news：相关资讯（辅助数据源）

**背景**：评估雪球做辅助数据源——实测无免登录公开接口（WAF 反爬），改为**东财资讯搜索**（免登录实测可用），合规零风险。

**需求/设计**：`features/news/SRS.md` + `features/news/SDD.md`

**新增**：
- `internal/news`（dao 层）：东财资讯搜索（JSONP 解析 + `<em>` 标签清洗）
- `GET /api/news?keyword=&count=`（count clamp 1-10，默认 5）+ `NewsProvider` 接口
- 前端「相关资讯」卡片（AI 分析之后）：标题链接 + 日期 + 来源；失败**静默隐藏**
- 资讯为展示数据，**不进 LLM 上下文**（合规，与 K线同策略）

**验证**：茅台 5 条真实资讯 ✅、参数校验 400 ✅、kline/resolve 回归 ✅、lint/test/JS 语法全绿 ✅

---

## 2026-08-16 · K线面板交互（悬停/触摸查看当日明细）

**设计**：`features/kline/INTERACTION.md`（命中检测 / 坐标换算 / 浮层规格 / 验收）

**实现**（纯前端，后端零改动）：
- `web/js/kline.js`：十字光标（虚线横竖线）+ 命中蜡烛高亮 + 明细浮层（日期/开收高低/成交量/涨跌幅，红涨绿跌）
  - 命中检测 O(1)：`idx = clamp(floor((px-padL)/cw), 0, n-1)`
  - viewBox↔CSS 坐标换算；浮层防溢出自动翻转
  - 移动端：touchstart 锁定浮层、click 切换锁定；`touch-action:none`
- `web/css/style.css`：`#kline-chart` 相对定位 + `.kline-tip` 浮层样式（明暗主题自适应）
- `web/js/app.js`：`renderKline(candles, quote)` 传报价（首根涨跌幅计算）

**验证**：8 个 JS 文件 node 语法检查全过 ✅、静态资源 200 ✅、API 回归 ✅

---

## 2026-08-16 · 前端分层：独立 web/ 文件夹

**背景**：前端从单文件 `internal/web/index.html`（内联 CSS/JS）重构为独立分层的 `web/` 文件夹。

**结构**：
```
web/
├── index.html          # 页面骨架（外链 css/js，无内联样式脚本）
├── css/style.css       # 设计系统（明暗主题变量）+ 组件样式
├── js/
│   ├── util.js         # 工具（$ / esc）
│   ├── theme.js        # 主题切换
│   ├── api.js          # API 层（resolve / kline / ask）
│   ├── sse.js          # SSE 协议解析
│   ├── markdown.js     # AI 分析 markdown 渲染
│   ├── kline.js        # K线 SVG 蜡烛图
│   ├── ui.js           # 展示层（报价卡/情绪面板/错误）
│   └── app.js          # 入口（视图/事件/状态）
└── embed.go            # go:embed 内嵌（index.html css js）
```

**变更**：
- `internal/web` 移除 → 顶层 `web/` 独立文件夹（与 cmd/internal 平级）
- `server.New` 签名：`indexHTML []byte` → `frontend fs.FS`；Routes 用 `http.FileServer` 提供 index.html + 静态资源
- `internal/server/handler_index.go` 删除（FileServer 接管）

**验证**：首页外链 9 个静态资源全 200 ✅、resolve/kline 200 ✅、SSE 全链路（847 delta）✅、lint/test 全绿 ✅

---

## 2026-08-16 · feature/kline：K线图与股价信息（实现完成）

**需求/设计**：`features/kline/SRS.md` + `features/kline/SDD.md`（分支内评审通过）

**新增**：
- `internal/kline`（dao 层）：腾讯接口单请求获取日K线 + 实时报价（前复权）
- `GET /api/kline?code=&market=&days=`（days clamp [10,250]，默认 60）
- 前端：股价卡（最新价/涨跌幅红涨绿跌/今开最高最低昨收成交量）+ **SVG 蜡烛图**（MA5/10/20 均线 + 成交量柱 + 坐标轴，明暗主题自适应）
- 行情经 `stock` 事件触发异步拉取，与 SSE 分析流并行，失败降级不影响情绪/AI

**实测发现并修复**：
- 东财接口密集请求后被限流（持续 EOF）→ 改用腾讯（单请求稳定）
- 腾讯除权除息日 K线行尾带**分红对象**（`{"FHcontent":"10派280.242元"}`）→ `[][]string` 解析炸，改用 RawMessage 宽容解析
- Go 默认 UA 曾被拒 → 请求带浏览器 UA

**验证**：茅台 60 根K线+报价（1341.99/-0.98%）✅、比亚迪 30 根 ✅、参数校验 400 ✅、SSE 回归 ✅、lint/test 全绿

---

## 2026-08-16 · M3 收尾：正式 README

- 替换 GitHub 自动生成的占位 README（原仅两行）
- 完整覆盖：功能特性 / 技术栈 / 快速开始（密钥配置三步）/ API 文档 / 项目结构 / 文档链 / 路线图 / 免责声明

---

## 2026-08-16 · 交互优化：不存在的股票友好降级

**问题**：输入不存在的股票时，`/api/ask` 返回原始技术错误（`[NodeRunError] failed to stream tool call...`），前端直接展示英文报错，不友好。

**修复**：
- `internal/agent`：`resolve_stock` 工具对「未找到」从**硬错误**改为返回结构化标记 `{"found":false,"message":"未找到该股票，请检查名称或代码"}`（业务降级，不让错误冒泡）
- `internal/agent`：Instruction 增加「found=false 时直接回复 message 并结束，不继续调工具」
- 前端 `showError`：改为友好文案「😥 出错了，请稍后重试」+ 技术细节弱化为小字

**验证**：
- 不存在的股票 → 流式 delta 优雅回复「未找到该股票，请检查名称或代码...」（无 error 事件）✅
- 正常股票（比亚迪）→ stock/sentiment/delta 全链路正常 ✅

---

## 2026-08-16 · ✅ 全链路真实联调成功（里程碑）

**状态**：填入真实密钥后，完整链路端到端验证通过。

**实测结果（`/api/ask` 茅台，总耗时 10.4s）**：
```
event: stock      → 贵州茅台 600519 沪A（东财识别 276ms）
event: sentiment  → heat=10 sample=10 多空=10%/50%/40% 强度=5 分，5 条真实知乎观点
event: delta×N    → 流式分析面板：情绪面总结 / 值得关注的讨论点 / 风险提示 / 来源引用
event: done
```

**验证点**：
- 知乎 `zhihu_search` Bearer 鉴权 ✅（返回真实讨论：茅台半年报、汇金退出、段永平）
- DeepSeek 批量情感分类 ✅（多空占比与内容一致）
- 强度分规则 ✅（空方 50% → 5 分中等偏弱，与公式预期一致）
- 流式 SSE ✅（逐字增量）；合规过滤 ✅（全文无禁用词，含「不构成投资建议」式措辞）
- 日志脱敏 ✅（启动日志显示 `68****c1` / `sk****db`）

**密钥安全**：`config.yaml` 权限 600、已被 .gitignore 排除、未入库。

---

## 2026-08-16 · v0.4.1 对齐知乎 house style（语义 + 接口 + 断言）

**背景**：对照 CursorPJ 下 org-go / user-core 的分层，评估结论「结构已 80% 对齐，补三件小事」。

**变更**：
- 语义对齐：`internal/agent`=controller（编排）、`internal/sentiment`=service、`internal/zhihu`/`internal/stock`=dao（包注释 + 文档对照表）
- 补 `sentiment.Searcher` 接口（消费方定义，`*zhihu.Client` 隐式实现）+ fake mock 单测（搜索失败/空结果降级路径，此前无密钥无法测）
- 编译期断言：`var _ Searcher = (*zhihu.Client)(nil)`、`var _ server.Analyzer/Resolver = (adapter)(nil)`

**未照搬**（有意保留）：全局 Default 单例（用显式注入）、`pkg/` 对外（用 `internal/`）、`controller/impl` 拆包（小项目不必要）。

**验证**：build / vet / test 全过（sentiment 测试增至 4 个用例）。
**文档同步**：`tech-design.md` §2.1 新增「与知乎 house style 对照」表。

---

## 2026-08-16 · v0.4.0 Router/Handler/Service 分层

**背景**：评审认为入口应分层（router/handler/service），本次按四层改造。

**变更**：
- 新增 `internal/server`（HTTP 层）：`Analyzer`/`Resolver` 接口 + `Server.New` + `Routes` + 3 个 handler + `response.go`
- `cmd/server/main.go` 瘦身：从 ~190 行降到 ~90 行，只留「配置 → buildDeps → server.New → 启动」
- 依赖倒置：handler 依赖接口，实装经适配器（`analyzerFunc`/`resolverFunc`）注入
- 哨兵错误移入 `internal/types`（`ErrStockNotFound`/`ErrEmptyQuery`），避免 handler 反向依赖 data 层
- 新增 `internal/server/server_test.go`：httptest + mock 接口实现，6 个用例（resolve 200/404/400、ask SSE、index）

**验证**：build / vet / test 全过（含新 server 测试）；探针 200、首页 200、无密钥降级路径一致。
**文档同步**：`tech-design.md` v1.2→v1.3（§2.1 目录/依赖/分层调用、§3.1 模块表）。

---

## 2026-08-16 · CR 修复（评审轮）

**问题与修复**：
- 🟡 `internal/agent`：流式分支中 `sink` 出错时 `return` 跳过 `MessageStream.Close()`（资源泄漏）→ 改为内层函数保证任何返回路径都 Close
- 🟢 `internal/{stock,zhihu,sentiment}`：`truncate` 按字节截断可能切断 UTF-8 字符 → 改为 rune 安全截断
- 🟢 `tech-design.md`：§2/§4/§6 残留 `static/index.html` 旧路径 → 更新为 `internal/web/index.html`

**验证**：build / vet / test 全过。

---

## 2026-08-16 · v0.3.0 项目结构重构（标准 cmd/internal 布局）

**背景**：原实现为扁平布局（9 个 .go 全在根目录），评审认为不工整；本版升级为标准 Go 项目布局，代码与文档同步。

**结构变更**：
```
zhihuDP/
├── cmd/server/main.go        # 入口：config.Load → buildDeps 组装 → 路由；CLI 模式(-q)
├── internal/
│   ├── config/               # Config / Load（yaml + env 覆盖 + 脱敏）
│   ├── types/                # StockInfo / SentimentResult / Ratio / ViewItem / Event
│   ├── stock/                # Resolve（东财主 + 腾讯兜底）
│   ├── zhihu/                # Client.New + (c *Client).Search
│   ├── sentiment/            # Analyze(ctx, code, name, zh, ds)（依赖注入）
│   ├── agent/                # Deps + RunAnalysis(ctx, query, deps, sink)
│   ├── compliance/           # Filter / FilterFinal
│   └── web/                  # go:embed 内嵌前端
└── docs/CHANGELOG.md
```

**要点**：
- 消除全局 `cfg` 变量 → `cmd/server.buildDeps` 显式依赖注入（`zhihu.Client` → `sentiment.Analyze` 闭包 → `agent.Deps`）
- 工具闭包直接捕获 `sink`，去掉 context 传事件的写法
- 前端 `//go:embed` 内嵌，不依赖运行目录（`static/` 移除）
- 单测拆分为 `internal/sentiment`、`internal/compliance` 包级测试

**验证**：`go build ./...` / `go vet` / `go test` 全过；探针 200、首页 200、无密钥降级路径一致。
**文档同步**：`tech-design.md` v1.1→v1.2（§2.1 目录/依赖/配置流向、§3.1 模块表、§3.2 方法归属）。

---

## 2026-08-16 · 文档基线（前置）

**交付物**：三步文档链全部完成，旧 `design.md`（问答 agent 方案）废弃。

| 文档 | 内容 | 版本 |
|---|---|---|
| `business-design.md` | 业务定位、业务规则 4 条（强度分/多空占比/热度/降级语义）、合规红线、8 项业务验收标准 | v1.0 |
| `product-design.md` | 搜索页/详情页线框、情绪面板/分析面板、降级路径、禁用词表 | v0.1 |
| `tech-design.md` | 架构定义（目录/依赖/并发/配置流/前端/运行形态）、依赖锁定表、8 项待确认 | v1.1 |

**关键决策**：
- 产品定位从「AI 荐股」转向「情绪数据工具」（合规红线：不输出买卖结论/不用概率/不自动交易/信息工具定位）
- 配置改为**配置文件模式**（`config.yaml`，用户自配密钥，`.gitignore` 排除）
- demo 不做行情面（知乎接口含部分行情数据，阶段 2 复用）

---

## 2026-08-16 · v0.0.1（M0）项目骨架 + 配置/搜索客户端

**对应提交**：`f8abc88`

**新增**：
- `go.mod`：模块 `zhihudp`；锁定 `eino v0.9.14` + `eino-ext/components/model/deepseek v0.1.7` + `yaml.v3 v3.0.1`（API 全部源码核对）
- `config.go`：配置文件加载（YAML + 环境变量覆盖 + 默认值 + 脱敏 `String()`）
- `stock.go`：股票识别客户端（东财 suggest 主选 + 腾讯 smartbox 兜底）
- `zhihu.go`：知乎 `zhihu_search` 客户端（Bearer + X-Request-Timestamp + Content-Type）
- `main.go`：`GET /api/resolve` 探针 + 首页占位 + 结构化日志
- `config.example.yaml` 模板、`.gitignore`

**修复（实测发现）**：
- 腾讯 smartbox 返回**非标准 JSON**（`v_hint="..."`，无匹配为 `v_hint="N";`，多结果 `^` 分隔）→ 改为手工解析，优先 A 股条目
- 两数据源均无匹配时从 502 改为 404（`ErrStockNotFound` 语义化）

**验证**：6/6 用例通过（名称/代码/深市/空参/不存在）。

---

## 2026-08-16 · v0.1.0（M1）情绪分析 + eino Agent

**对应提交**：`13cec37`

**新增**：
- `sentiment.go`：`AnalyzeSentiment` 编排（知乎搜索 → LLM 批量情感分类 → 规则强度分）；**全环节降级**（样本<5/搜索失败/缺密钥 → `Degraded=true` 而非报错）
- `agent.go`：eino ADK `ChatModelAgent`（ReAct）+ 2 个工具（`resolve_stock` / `analyze_sentiment`）；事件经 context 注入工具层直发 `stock`/`sentiment` 事件；`MessageStream.Recv()` 循环提取流式 delta
- `types.go`：`SentimentResult` / `Ratio` / `ViewItem` / `Event` 共享结构
- `core_test.go`：强度分/合规过滤/无密钥降级单测

**变更/校准**：
- **强度分公式定稿**：原草案在「样本多但多空分歧大」时虚高（五五开得 8 分）→ 改为一致性主导 `1 + 6*max(bull,bear) + log10(1+sample)/2 + min(heat,50)/100`（五五开≈5 分、强一致≈7-9 分）；`tech-design.md` 同步

**验证**：`go build`/`go vet` ✅；单测全过 ✅。

---

## 2026-08-16 · v0.2.0（M2）Web 层 SSE + 前端两页

**对应提交**：`63b8290`

**新增**：
- `main.go`：`POST /api/ask` SSE 流式（`stock`/`sentiment`/`delta`/`done`/`error` 五事件）+ `-q` CLI 模式 + 静态托管
- `compliance.go`：禁用词过滤（流式块级 `Filter` + 完整文本句级 `FilterFinal`）
- `static/index.html`：搜索页 + 详情页（情绪面板：热度/多空占比/参考强度/代表观点；AI 分析流式展示），`fetch` + `ReadableStream` 手写 SSE 解析，无构建

**验证**（无密钥可验部分）：
- `/api/resolve?q=茅台` → 200 `贵州茅台 600519 沪A`
- `/` 首页 → 200
- `/api/ask` 无密钥 → SSE `error` 事件（提示填密钥）
- CLI `-q 茅台` → 正常输出

---

## 2026-08-16 · 收尾

**对应提交**：`06aba71`

- 补充 `go.mod`/`go.sum` 依赖声明（M1 引入 eino 后 tidy）
- 初始化 Git（`main` 分支）、创建公开仓库并推送：https://github.com/ZZ2416/zhihuDP
- 全库密钥扫描：`config.yaml` 排除、无硬编码密钥

---

## 待办（未完成项）

- [ ] 用户填入密钥后全链路联调（`config.yaml`）
- [ ] M3 打磨：正式 README、免责声明完善、错误收口
- [ ] 阶段 2：行情数据实测（知乎接口）、情绪分类正式评测基准（100 条抽样 ≥85%）
