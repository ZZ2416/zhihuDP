# 恢复知乎情绪分析（与基本面并存）— 软件设计文档（SDD）

> 版本：v0.1 · 日期：2026-08-19 · 分支：`feature/emotion`
> 依据：`features/emotion/设计方案.md`（v0.1）
> 决策：情绪/基本面**各自独立解读**（两次 LLM）· 知乎密钥**配置+弹窗恢复** · 情绪面板在**基本面卡之上**

---

## 1. 概述

恢复此前删除的知乎情绪分析（zhihu_search + LLM 分类 + 强度分），与基本面分析并存：ask 事件增加 `sentiment`（情绪数据），新增独立 `POST /api/emotion/analyze`（情绪 AI 解读 SSE），前端恢复情绪面板卡（基本面之上）。

## 2. 总体架构

```
恢复：internal/zhihu（zhihu_search 客户端）、internal/sentiment（情绪分析服务）、config.zhihu、types 情绪类型
新增：internal/server/handler_emotion.go（POST /api/emotion/analyze SSE）
新增：internal/agent/emotion.go（情绪解读 prompt + 流式，与 finance.go 同模式）
改造：agent.go（analyze_sentiment 工具发 sentiment 事件）、handler_ask（捕获 sentiment）、chat（上下文加情绪）、main（zhClient）、前端（情绪面板 + 弹窗知乎字段）
```

## 3. 数据结构（types 恢复）

```go
// 从 git 历史恢复（7722088 删除前）：
type Ratio struct { Bull, Bear, Neutral float64 }        // 多空占比（0-1，和为 1）
type ViewItem struct { Title, Url string }               // 代表观点（带原帖链接）
type SentimentResult struct {
    Code, Name string
    Heat       int       // 讨论量（本次取回条数）
    Sample     int       // 实际分类样本数
    Ratio      Ratio
    Score      *int      // 参考强度 1-10；样本不足 nil
    Items      []ViewItem // 代表观点 ≤5
    Degraded   bool
    ErrMsg     string
}
```

## 4. API 设计

### /api/ask 事件协议（改造）
```
stock（识别）→ sentiment（情绪数据 SentimentResult）→ fundamental（四维评分）
→ delta×N（基本面解读）→ done / error
```
- `sentiment` 事件：完整 SentimentResult JSON；前端渲染情绪面板 + **自动触发情绪解读**

### POST /api/emotion/analyze（新增，SSE）
- body `{code, market}`（market 仅用于识别，服务端按 code 取情绪）
- 流程：服务端 `sentiment.Analyze` 拉情绪 → 注入 LLM → `delta`×N（情绪解读）→ `done`/`error`
- **计配额**（与 ask/chat/finance-analyze 共享 20 次/页）；失败友好降级（面板"暂不可用+重试"）

## 5. 情绪分析流程（internal/sentiment 恢复）

```
sentiment.Analyze(ctx, code, name, zhClient, deepseekCfg) → *SentimentResult
  1) zhihu_search 搜索（name，近 30 天，取 10 条）——失败 → Degraded=true
  2) LLM 批量情感分类（classifyPosts：逐条看多/看空/中性）
  3) 热度=取回条数；多空占比=分类计数归一；参考强度=ComputeStrength（规则：一致性×样本）
  4) 代表观点 ≤5（按强度排序）
  任何环节失败降级（Degraded=true + ErrMsg），不返回 error（agent 走降级文案）
```

## 6. 情绪解读（internal/agent/emotion.go，新增）

- 与 `finance.go` 同模式：`newDeepSeekModel` + Stream + compliance 过滤（含**非 EOF 错误上报**）
- prompt：注入情绪数据（热度/多空占比/参考强度/代表观点标题）→ 输出 3 段：
  1. 情绪面总结（多空分布 + 参考强度）
  2. 值得关注的讨论点（引用代表观点，带链接）
  3. 风险提示
- 合规：不荐股、无概率词；文末「数据来源知乎公开讨论，不构成投资建议」
- 降级：数据 Degraded 时如实说明（"讨论较少/搜索受限"）

## 7. 时序图

```plantuml
@startuml
Browser -> server: POST /api/ask {stock}
server -> internal/agent: RunAnalysis（resolve → analyze_sentiment → analyze_fundamental）
internal/agent -> zhihu.Search: 知乎搜索（近30天）
internal/agent -> deepseek: 批量情感分类
internal/agent -> server: event sentiment（SentimentResult）
internal/agent -> fundamental: 四维评分
internal/agent -> server: event fundamental
internal/agent -> deepseek: 基本面解读
server --> Browser: SSE delta(基本面)...done

Browser -> server: 收到 sentiment → POST /api/emotion/analyze（计配额）
server -> sentiment.Analyze: 拉情绪（复用/重算）
server -> agent.EmotionAnalyze: 情绪解读 prompt + 流式
server --> Browser: SSE delta(情绪解读)...done
@enduml
```

## 8. 详细设计

### 8.1 zhihu 客户端（internal/zhihu 恢复）
- `Client{cfg config.ZhihuConfig}`（Bearer 鉴权 + `X-Request-Timestamp`）；`Search(ctx, query, count)` → `SearchResponse{Data.Items[{Title, Url,...}]}`
- 密钥热更新：`UpdateKeys`（恢复，弹窗知乎字段用）

### 8.2 sentiment 服务（internal/sentiment 恢复）
- `Analyze(ctx, code, name, s Searcher, ds config.DeepSeekConfig) (*types.SentimentResult, error)`
- 依赖注入 `Searcher` 接口（*zhihu.Client 实现，mock 可测）
- `ComputeStrength(ratio, sample, heat)` 规则强度分（单测保留原用例）

### 8.3 agent 改造（analyze_sentiment 工具）
- `Deps` 加 `AnalyzeSentiment func(ctx, code, name) (*types.SentimentResult, error)`（main 注入 `sentiment.Analyze` 包装）
- 工具闭包：调 AnalyzeSentiment → sink 发 `sentiment` 事件 → 返回 JSON（**降级 JSON**：失败返回 `{"degraded":true,"err_msg":...}`，不终止 agent）
- Instruction：流程 `resolve_stock → analyze_sentiment → analyze_fundamental → 撰写基本面解读`（情绪数据供基本面解读参考；情绪独立解读走 /api/emotion/analyze）

### 8.4 handler_ask（改造）
- 捕获 `sentiment` 事件（`*types.SentimentResult`）→ 快照存 chat（情绪摘要供对话上下文）

### 8.5 chat（改造）
- `ChatFacts` 加 `Sentiment`（多空/热度/参考强度/代表观点摘要）
- `Snapshot` 加 `Sentiment *types.SentimentResult`；`SetSnapshot` 签名恢复 sentiment 参数

### 8.6 handler_emotion（新增）
- `POST /api/emotion/analyze`：配额扣减 → `sentiment.Analyze` → `agent.EmotionAnalyze`（SSE，复用 writeSSE/flusher 模式）

### 8.7 密钥（配置 + 弹窗恢复）
- `config.ZhihuConfig` 恢复（access_secret / access_secret_enc / base_url）
- `PersistEnc` 恢复 zhihuEnc 写回；`decryptEncKeys` 解密 zhihu enc
- 前端弹窗加回知乎字段（`key-zhihu`），keys.js 加密两字段提交（RSA-OAEP）

### 8.8 前端（情绪面板，基本面卡之上）
- 详情页：`fundamental-card` 前插入 `sentiment-card`（热度/多空占比条形/参考强度/代表观点列表 + 情绪解读区）
- `handleEvent` 加 `sentiment` 分支：`renderSentiment(d)`（恢复原渲染）+ 自动 `analyzeEmotion(code, market)`（POST /api/emotion/analyze SSE 流式渲染）
- 失败：面板"情绪数据暂不可用 + 重试"（不阻塞基本面）
- 卡片结构在 `#finance-card`（财务解析）之后、`#fundamental-card` 之前

## 9. 文件改动清单

| 文件 | 动作 |
|---|---|
| internal/zhihu/zhihu.go + _test.go | 恢复（Search + UpdateKeys） |
| internal/sentiment/sentiment.go + _test.go | 恢复（Analyze/classifyPosts/ComputeStrength） |
| internal/types/types.go | 改：恢复 Ratio/ViewItem/SentimentResult；ChatFacts/Snapshot 加 Sentiment |
| internal/agent/emotion.go | 新增：情绪解读 |
| internal/agent/agent.go | 改：Deps.AnalyzeSentiment + analyze_sentiment 工具 + Instruction |
| internal/agent/chat.go | 改：formatFacts 加情绪 |
| internal/chat/chat.go | 改：Snapshot/Deps/SetSnapshot 加情绪 |
| internal/server/handler_emotion.go | 新增：POST /api/emotion/analyze |
| internal/server/handler_ask.go | 改：捕获 sentiment 事件 + 快照 |
| internal/server/server.go | 改：路由 + EmotionProvider 接口（或复用注入函数） |
| internal/config/config.go + config.example.yaml | 改：恢复 zhihu 配置 |
| cmd/server/main.go | 改：zhClient 组装、Deps 注入、弹窗密钥 |
| web/index.html + web/js/{ui,app,keys}.js + css | 改：情绪面板 + 弹窗知乎字段 + 渲染函数 |
| features/emotion/SDD.md | 本文件 |
| docs/CHANGELOG.md | 改 |

## 10. 验证方案

1. 单测：zhihu 解析、sentiment（分类降级/强度分边界，恢复原用例）、emotion handler
2. 接口冒烟：ask 事件含 sentiment+fundamental；emotion/analyze SSE 流式；知乎密钥生效（真实数据）
3. 合规：情绪解读无买入/概率词；参考强度 1-10
4. 回归：基本面分析、看山对话（含情绪上下文）、密钥弹窗（两字段 RSA）、分时/财报/资讯
5. 全量 `go build/vet/test -race`

## 11. 上线三板斧

1. **灰度**：本机真实知乎密钥（情绪面板真实数据 + 情绪解读）→ 对照原一期行为
2. **监控**：`[emotion]` 日志（搜索/分类耗时、降级原因）；知乎接口失败观察
3. **回滚**：删除 `/api/emotion/analyze` 路由 + ask 去掉 sentiment 事件即退化为纯基本面（前后端独立）
