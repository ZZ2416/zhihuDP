# 二期 · 看山追问对话（AI 看山 Agent）— 软件设计文档（SDD）

> 版本：v0.1 · 日期：2026-08-16 · 分支：`v2_chat`（规划）
> 依据：`features/v2/SRS.md`（需求，v0.2 已定稿）

---

## 1. 概述

在详情页 **K 线面板下方**增加「与看山对话」独立对话区：用户基于一期结果（行情快照 / 情绪面板 / 知识库 / 前次 AI 分析）继续追问，agent 以 **AI 看山**人设多轮回答，尺度限定为方法/维度/风险提示（**不荐股**）。会话按股票隔离，一期分析保持中性文案不变。

## 2. 总体架构（分层归属）

```
internal/types（新增 ChatMessage / ChatSession / ChatFacts）
internal/chat（service 层，新增）—— 会话存储（按股票隔离）+ 上下文事实组装
internal/agent（chat.go，新增）  —— 看山人设 system prompt + 流式 LLM 对话 + 合规过滤
internal/server（handler_chat.go + ChatProvider 接口）—— POST /api/chat（SSE）
web/index.html —— K 线面板下方对话区结构
web/js/chat.js（新增）—— 对话渲染 / 发送 / SSE 增量 / 清空
web/js/{app,ui}.js —— 会话初始化与切股重置
```

新增依赖注入链（`server.New` 9 参）：
```
cmd/server.main → chatProvider{store + agent.Chat} → server.ChatProvider
```

## 3. 数据结构（internal/types 新增）

```go
// ChatMessage 对话消息（会话历史）
type ChatMessage struct {
    Role    string `json:"role"`    // user / assistant
    Content string `json:"content"`
}

// ChatFacts 对话上下文事实快照（组装后注入 LLM）
type ChatFacts struct {
    StockName    string  // 股票名
    StockCode    string
    Sector       string  // 所属板块（如 沪A / 行业）
    Quote        string  // 行情快照文本：现价/涨跌幅/今开/最高/最低/成交量
    Sentiment    string  // 情绪面板摘要文本（heat/ratio/score/代表观点）
    Knowledge    string  // 知识库检索片段（截断 ≤600 字）
    PrevAnalysis string  // 前次 AI 分析文本（一期中性文案，原样引用）
}
```

## 4. API 设计

### POST /api/chat（SSE 流式）

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| stock | string | 是 | 股票**代码**（如 600519），会话按此隔离 |
| market | string | 否 | 市场（沪A/深A），用于实时取行情快照 |
| message | string | 是 | 用户追问（非空，≤500 字） |
| session_id | string | 否 | 预留；默认服务端按 stock 维护单一会话 |

- 请求示例：`{"stock":"600519","market":"沪A","message":"情绪为什么偏空？"}`
- 响应（SSE，复用一期事件协议）：
  - `event: delta` `data: {"text":"看山觉得…"}`（增量，逐段）
  - `event: done`（收尾）
  - `event: error` `data: {"message":"…"}`（异常/合规拦截说明）
- 失败：非 SSE 的 400（参数非法）/ 502（LLM 或上游失败）

### 会话语义
- 首次对某股票提问 → 服务端创建该股票会话（**与一期分析快照绑定**）
- 再次提问 → 沿用同一会话（历史累加，窗口最近 10 条）
- 切换股票提问 → 各自独立会话，互不串扰
- 前端「清空」→ 删除该股票会话，重新开始

## 5. 时序图

```plantuml
@startuml
Browser -> server: POST /api/chat {stock, message}
server -> internal/chat: GetOrCreate(stock) + 追加用户消息
internal/chat -> internal/agent: Chat(ctx, facts, history, message)
internal/chat -> zhihu: KnowledgeSearch(股票/个股名)  // 检索片段（复用一期）
internal/agent -> deepseek: 人设 system prompt + facts + history + message
deepseek --> internal/agent: 流式输出
internal/agent -> compliance: Filter(delta)          // 禁荐股/概率词
internal/agent --> server: delta 事件
server --> Browser: SSE delta...done
server -> internal/chat: 追加助手回复到会话
@enduml
```

## 6. 详细设计

### 6.1 会话与快照（internal/chat）
```go
type Store struct {
    mu        sync.Mutex
    sessions  map[string]*Session    // key: stock code
    snapshots map[string]Snapshot    // key: stock code，一期 /api/ask 结果快照
    historyN  int                    // 历史窗口，默认 10
}
type Snapshot struct {
    Sentiment *types.SentimentResult // 情绪面板结果
    Analysis  string                 // 一期 AI 分析最终文本（中性文案）
}
type Session struct {
    Stock    types.StockInfo
    Messages []types.ChatMessage
}
```
- **快照来源（CR 修正）**：`handler_ask` 的 sink 捕获 `stock` 事件（拿 code）、`sentiment` 事件（`*SentimentResult`）、`delta` 事件（累积分析文本）→ 分析结束后 `chat.SetSnapshot(code, sentiment, analysis)`；最近一次 ask 覆盖旧快照
- 纯内存，重启即失（demo 可接受，文档注明）；`GetOrCreate(code)` 首次时若快照存在则初始化 Session / `Reset(code)` / `Append(code, msg)`

### 6.2 上下文事实组装（BuildFacts）
- **行情快照（CR 修正）**：对话时通过注入的 kline 函数实时取 `types.Quote`（`GetKline(market, code, 1)` 只取 Quote 段，不取 K 线序列）→ 文本化（现价/涨跌幅/今开/最高/最低/成交量）
- 情绪面板：`Snapshot.Sentiment`（heat/sample/ratio/score/代表观点 ≤5 条标题）
- 知识库：复用 `zhClient.KnowledgeSearch`，查询「股票名 + 关键问句」取 3 条片段，截断 ≤600 字
- 前次分析：`Snapshot.Analysis`（一期中性文案，原样引用）
- 事实注入顺序：股票 → 行情 → 情绪 → 知识库 → 前次分析 → 历史 → 新提问

### 6.3 AI 看山人设（agent/chat.go system prompt 骨架）
```
你是「AI 看山」，知乎吉祥物刘看山（北极狐）的化身，擅长以温和、拟人的口吻聊股票投资话题。
规则：
1. 自称「看山」；回答克制、专业，不编造数据，只基于提供的上下文事实；
2. 不做任何投资决策：不推荐/暗示买卖、不给目标价、不用「概率/胜率」措辞；
3. 被问「该买吗/该卖吗/抄底/加仓」时，转向方法、关注维度、风险提示，并说明不荐股；
4. 上下文不足时明确说明，不臆测。
```
- 与一期 `RunAnalysis` 的模型共用 `deps.DeepSeek()`（同一 DeepSeek 配置，密钥热更新同样生效）
- 复用 eino deepseek 流式：`model.Stream` → 每段过 `compliance.Filter` → delta 事件；结束过 `FilterFinal`

### 6.4 前端（K 线面板下方对话区）
- 结构：头像（`/pic/kanshan.jpg`）+ 消息列表 + 输入框 + 「发送/清空」
- `chat.js`：`sendChat(stock, msg)` 复用一期 POST SSE 读取（`sse.js` 的 fetch 流式）；增量渲染；错误提示 + 重试
- 切股重置：`doSearch` 成功后清空对话区并标记当前股票（会话按 stock 天然隔离，前端仅做展示重置）
- 样式复用 `.card` / markdown 渲染；看山回复左侧头像，用户消息右侧

## 7. 文件改动清单

| 文件 | 动作 | 说明 |
|---|---|---|
| internal/types/types.go | 改 | ChatMessage / ChatSession / ChatFacts |
| internal/chat/chat.go | 新增 | 会话存储 + BuildFacts（依赖 zhihu/agent 注入） |
| internal/agent/chat.go | 新增 | 看山人设 prompt + 流式对话 + 合规 |
| internal/server/server.go | 改 | ChatProvider 接口 + New 第 9 参 + 路由 |
| internal/server/handler_chat.go | 新增 | POST /api/chat（SSE）+ 参数校验 |
| cmd/server/main.go | 改 | 组装 chatProvider（store + agent.Chat） |
| web/index.html | 改 | 详情视图 K 线卡片下加对话区 |
| web/js/chat.js | 新增 | 对话渲染/发送/增量/清空 |
| web/js/{app,ui}.js | 改 | 会话展示重置、样式 |
| web/css/style.css | 改 | 对话区样式 |
| docs/CHANGELOG.md | 改 | 二期里程碑 |
| features/v2/SRS.md | 定稿 | 已更新决策记录 |

## 8. 验证方案

1. 单元测试：会话隔离（两股票互不串扰）、BuildFacts 格式化、合规过滤拒绝「该买吗」
2. 接口冒烟：POST /api/chat 三轮追问上下文连贯；诱导提问被合规拦截
3. 前端：K 线下对话区渲染、SSE 打字机、清空、切股重置
4. 回归：一期 /api/ask、热门、知识库、密钥弹窗不受影响
5. 全量 `go build ./... && go vet ./... && go test ./...`

## 9. 上线三板斧

1. **灰度**：先本机全链路冒烟（真实密钥）→ 浏览器实测追问 3 轮
2. **监控**：`[chat]` 日志（耗时/轮次/合规拦截次数）；LLM 超时错误统计
3. **回滚**：删除 `/api/chat` 路由即整体下线二期对话，一期不受影响（前后端独立）
