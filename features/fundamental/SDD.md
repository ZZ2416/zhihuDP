# 基本面分析（替换知乎情绪，定位 v2）— 软件设计文档（SDD）

> 版本：v0.2（决策定稿）· 日期：2026-08-19 · 分支：`feature/fundamental`
> 决策补充：① AI 解读**合并进 ask**（去掉独立 /api/fundamental/analyze）② 人设改为中性助手 + **「黑德文」**黑色系标识（非拟人）③ 估值分位用 **PE(TTM) 分位**（PB 仅展示）
> 依据：`business-design.md`（定位 v2）、`features/fundamental/设计方案.md`（v0.2）
> 本期范围：删知乎 → 四维评分（各自卡片）→ 估值历史分位 → 自选池 20 只；**产业链不做**

---

## 1. 概述

移除知乎情绪分析数据源，改为**多源基本面分析**：财务（东财，复用现有 finance）+ 估值（腾讯当前 + 东财历史分位）→ **四维评分模型**（盈利/成长/财务健康/估值）→ AI 解读。新增自选池（20 只）。产品定位从"情绪分析工具"转为"自用数据聚合平台（基本面方向）"。

## 2. 总体架构

```
internal/valuation（dao，新增）—— 腾讯 qt 估值（PE/PB/市值）+ 东财历史 PE 分位
internal/fundamental（service，新增）—— 四维评分模型（输入 finance 指标 + valuation）
internal/server（handler_fundamental.go + WatchlistProvider/FundamentalProvider）—— API
internal/agent —— ask 改造（基本面解读 prompt，去情绪）
internal/chat —— 上下文去 Sentiment/Knowledge，加 Valuation

删除：internal/sentiment、internal/zhihu、knowledge（server/chat/main/config/web）
```

注入链（`server.New` 第 15 参）：
```
cmd/server.main → fundamentalService{fiance: finance.GetResult, valuation: valuation.Get, score: fundamental.Score} → server.FundamentalProvider
```

## 3. 数据结构（internal/types 新增）

```go
// Valuation 估值（当前值 + 历史分位）
type Valuation struct {
    PE           float64 `json:"pe"`            // PE(TTM)
    PB           float64 `json:"pb"`            // PB(MRQ)
    MarketCap    float64 `json:"market_cap"`    // 总市值（亿元）
    PEFlow       float64 `json:"pe_flow"`       // PE(TTM)动态
    PEEntPercent float64 `json:"pe_ent_percent"`// 当前 PE 历史分位 0-100（无数据 -1）
    Degraded     bool    `json:"degraded"`
}

// FundamentalScore 四维评分
type FundamentalScore struct {
    Profit   int `json:"profit"`   // 盈利能力 0-100
    Growth   int `json:"growth"`   // 成长性
    Health   int `json:"health"`   // 财务健康
    Valuat   int `json:"valuation"`// 估值
    Total    int `json:"total"`    // 加权总分
    Grade    string `json:"grade"` // 质地强/良好/一般/偏弱
    NoData   []string `json:"no_data,omitempty"` // 数据不足的维度
}

// FundamentalResult 基本面聚合结果
type FundamentalResult struct {
    Code       string                `json:"code"`
    Name       string                `json:"name"`
    Indicators []FinancialIndicator  `json:"indicators"` // 复用（5年年报+最新期）
    Valuation  Valuation             `json:"valuation"`
    Score      FundamentalScore      `json:"score"`
    Degraded   bool                  `json:"degraded"`
}

// WatchItem 自选池条目
type WatchItem struct {
    Code   string `json:"code"`
    Market string `json:"market"`
    Name   string `json:"name"` // 加载时补全
}
```

## 4. API 设计

### GET /api/fundamental?code=600519&market=沪A
- 返回 `FundamentalResult`（指标 + 估值 + 四维评分）；展示数据，不计配额
- 失败 502（财务或估值全挂）

### 自选池
| 接口 | 说明 |
|---|---|
| `GET /api/watchlist` | 返回条目列表（附评分/价格/涨跌幅——前端再取或服务端聚合） |
| `POST /api/watchlist/add` `{code, market}` | 添加（上限 20，超出 400） |
| `POST /api/watchlist/remove` `{code}` | 移除 |

- 持久化：`watchlist.json`（config `server.data_dir`，默认 ./data/），本地文件，重启保留

### /api/ask 改造（唯一 AI 解读入口）
- 事件协议：`stock` → **`fundamental`**（评分 JSON，替代原 sentiment）→ `delta`×N（**AI 基本面解读**，注入评分+财报+估值）→ `done`
- **去掉独立 /api/fundamental/analyze**（避免两次 LLM 重复分析）；delta 计配额（ask 原机制）
- 移除 `sentiment` 事件与情绪相关工具/依赖

## 5. 四维评分模型（打分函数，规则可复现）

| 维度(权重) | 指标 | 打分（线性插值） | 权重内配 |
|---|---|---|---|
| 盈利 30% | ROE(加权) | ≥25→100 / 20→85 / 15→70 / 10→50 / 5→30 / ≤0→10 | 0.4 |
| | 毛利率 | ≥60→100 / 40→75 / 25→55 / 15→40 / ≤10→25 | 0.3 |
| | 净利率 | ≥30→100 / 20→80 / 10→60 / 5→40 / ≤0→20 | 0.3 |
| | 趋势调整 | 最近 ROE vs 5 年前：升≥5pt +10 / 降≥5pt −10，其余 0 | ±10 |
| 成长 25% | 5年营收CAGR | ≥30→100 / 20→80 / 15→65 / 10→50 / 5→35 / ≤0→15 | 0.5 |
| | 最近净利同比 | ≥30→100 / 15→80 / 5→65 / 0→50 / −10→30 / ≤−20→10 | 0.5 |
| 健康 25% | 资产负债率 | ≤20→100 / 30→85 / 40→70 / 50→55 / 60→40 / 70→25 / ≥80→10 | 0.6 |
| | 经营现金流/营收 | ≥0.5→100 / 0.3→80 / 0.15→60 / 0→40 / <0→20 | 0.4 |
| 估值 20% | PE 历史分位 p | p≤20→100；20<p<80 线性 100→30；p≥80→10；无分位→50+标注 | 1.0 |

- 总分 = `round(盈利×0.3 + 成长×0.25 + 健康×0.25 + 估值×0.2)`（数据不足维度剔除后权重归一）
- 定性：≥75 质地强 / 60-74 良好 / 40-59 一般 / <40 偏弱

## 6. 时序图

```plantuml
@startuml
Browser -> server: 搜索 → POST /api/ask {stock}
server -> internal/agent: RunAnalysis（去情绪，注入基本面）
internal/agent -> server: event stock（识别）
server -> internal/fundamental: Score(ctx, code, market)
internal/fundamental -> finance.GetResult: 5年年报+最新期
internal/fundamental -> valuation.Get: 腾讯PE/PB/市值 + 东财历史PE序列→分位
internal/fundamental --> server: FundamentalResult（评分）
server --> Browser: event fundamental（评分JSON，渲染评分卡片）
internal/agent -> deepseek: 基本面解读 prompt（注入评分+指标+估值）
deepseek --> internal/agent: 流式
server --> Browser: SSE delta...done

Browser -> server: POST /api/watchlist/add {code}（自选池，≤20）
server -> watchlistStore: 追加 + 写文件
@enduml
```

## 7. 详细设计

### 7.1 valuation dao（internal/valuation/valuation.go）
- **腾讯当前估值**：`qt.gtimg.cn/q=sh600519`（GBK 解码，`~` 分割）→ `[39]`PE、`[46]`PB、`[45]`总市值（亿）
- **东财历史分位**：`RPT_VALUEANALYSIS_DET`，`filter=(SECUCODE="600519.SH")`，`sortColumns=TRADE_DATE` 取最近 **419** 条 PE_TTM → 当前 PE 百分位（`(count(≤cur)/n)*100`）；失败 → 分位 -1 + Degraded
- 串行取（东财并发易触发风控，finance 同经验）；`Timeout 6s`；浏览器 UA

### 7.2 fundamental service（internal/fundamental/fundamental.go）
- `Score(ctx, code, market) (*types.FundamentalResult, error)`：调 finance.GetResult + valuation.Get → 评分模型
- 评分纯函数（无 IO），单测覆盖边界

### 7.3 agent 改造（ask 基本面解读）
- 删除 sentiment 工具与情绪注入
- Instruction 改为**中性专业**基本面解读（标识「黑德文」仅界面层，回答不拟人）：注入 评分（四维+总分+定性）+ 财务指标表 + 估值（PE/PB/分位）
- 输出五段（综合质地/盈利/成长/健康/估值风险），合规过滤不变
- 人设：去「AI 看山」口吻 → 中性分析助手；前端黑色系「黑德文」标识

### 7.4 chat 上下文改造
- `ChatFacts`：删 `Sentiment`/`Knowledge`；加 `Valuation`（估值+分位）与 `Score`（四维评分）
- chat.Service Deps：删 `Knowledge`；加 `Fundamental func(...) (*types.FundamentalResult, error)`（可 nil）

### 7.5 自选池（internal/watchlist）
- `Store{mu, items []WatchItem, max 20, path}`：Load（启动）/ Save（增删后写 JSON）；`Add`（去重+上限）/ `Remove` / `List`
- 列表展示：服务端返回条目，前端逐只调 `/api/fundamental` 聚合评分（或列表接口批量，MVP 前端循环，P1 再优化）

### 7.6 前端
- 详情页：删「情绪面板」「讨论文章」卡片；「财务解析」保留（表格）
- 新增「**基本面分析**」卡片（情绪面板原位置）：四维评分条（各带分数）+ 总分大数字 + 定性标签 + 估值行（PE/PB/分位）+ AI 解读（ask 的 delta 渲染到此处，或 fundamental/analyze）
- 主栏目：新增「**自选池**」卡片（添加/删除，列表展示评分+价格+涨跌，点击进详情）

## 8. 知乎移除清单

| 项 | 处理 |
|---|---|
| internal/sentiment（含测试） | 删除 |
| internal/zhihu | 删除 |
| config.zhihu（access_secret 等）+ config.example | 删除 |
| /api/knowledge + KnowledgeProvider + handler_knowledge | 删除 |
| /api/ask 的 sentiment 事件 + agent sentiment 工具 | 移除 |
| 前端情绪面板 / 讨论文章卡片 + 相关 js | 删除 |
| chat Sentiment/Knowledge 上下文 + Deps.Knowledge | 移除 |
| main 中 zhClient / knowledge 注入 | 移除 |
| server.New 参数收拢（移除 knowledge，新增 fundamental/watchlist） | 调整 |

## 9. 文件改动清单

| 文件 | 动作 |
|---|---|
| internal/valuation/valuation.go + _test.go | 新增 |
| internal/fundamental/fundamental.go + _test.go | 新增 |
| internal/watchlist/watchlist.go + _test.go | 新增 |
| internal/agent/agent.go | 改：去 sentiment，Instruction 基本面 |
| internal/agent/chat.go | 改：formatFacts 去 Sentiment/Knowledge 加 Valuation/Score |
| internal/chat/chat.go | 改：Deps/上下文 |
| internal/config/config.go | 改：删 zhihu，加 server.data_dir |
| internal/server/server.go | 改：接口/New/路由收拢 |
| internal/server/handler_fundamental.go / handler_watchlist.go | 新增 |
| internal/server/handler_ask.go | 改：事件捕获 fundamental、去 sentiment |
| internal/server/handler_knowledge.go | 删除 |
| cmd/server/main.go | 改：组装收拢 |
| web/index.html + css | 改：删情绪/讨论，加基本面/自选池 |
| web/js/{app,ui}.js + finance.js | 改 |
| web/js/fundamental.js / watchlist.js | 新增 |
| internal/sentiment/、internal/zhihu/ | 删除 |
| features/fundamental/SRS.md | 新增 |
| docs/CHANGELOG.md | 改 |

## 10. 验证方案

1. 单测：valuation（GBK 解析/分位计算）、fundamental 评分（各维边界/权重归一/数据不足）、watchlist（上限/去重/持久化）
2. 接口冒烟：600519 四维评分总分（对照手工算）、分位值；自选池增删查
3. 合规：诱导「基本面好该买吗」→ 拒绝 + 风险提示
4. 回归：ask（无 sentiment 事件、有 fundamental 事件）、chat（无知识库）、分时/密钥/媒体正常
5. 全量 build/vet/test

## 11. 上线三板斧

1. **灰度**：本机全链路（600519 评分对照东财估值页）+ 自选池增删
2. **监控**：`[fundamental]` 日志（分位源成功/失败、评分）；估值接口失败降级观察
3. **回滚**：git revert 本分支（删除项均为代码移除，回退即恢复知乎功能）
