# 财报爬取 + AI 解析 — 软件设计文档（SDD）

> 版本：v0.1 · 日期：2026-08-17 · 分支：`feature/finance`
> 依据：`features/finance/设计方案.md`（v0.1，已确认）
> 决策补充：AI 解析**自动触发**（财务数据加载后流式输出，与情绪面板/AI 分析一致）

---

## 1. 概述

搜索个股后，详情页新增「**财务解析**」卡片：抓取核心财务指标（5 年年报 + 最新报告期，东财双源互备），AI 流式解读盈利/成长/财务健康/风险（只解读不荐股）。

## 2. 总体架构

```
internal/finance（dao，新增）—— 东财财务客户端（主+兜底双接口，指标归一化）
internal/agent/finance.go（新增）—— 财务解析 prompt + 流式 LLM + 合规过滤
internal/server（handler_finance.go + FinanceProvider 接口）—— GET /api/finance + POST /api/finance/analyze
web/index.html + web/js/finance.js —— 财务卡片（表格 + AI 解析）
```

注入链（`server.New` 第 12 参）：
```
cmd/server.main → financeProvider{dao: finance.GetResult, agent: agent.AnalyzeFinance} → server.FinanceProvider
```

## 3. 数据结构（internal/types 新增）

```go
// FinancialIndicator 单报告期财务指标（数值已归一化为亿元 / %）
type FinancialIndicator struct {
    ReportDate     string  `json:"report_date"`      // 如 2025年报 / 2026中报
    ReportDateFull string  `json:"report_date_full"` // 如 2025-12-31
    Revenue        float64 `json:"revenue"`          // 营业总收入（亿元）
    RevenueYoY     float64 `json:"revenue_yoy"`      // 营收同比 %
    NetProfit      float64 `json:"net_profit"`       // 归母净利润（亿元）
    NetProfitYoY   float64 `json:"net_profit_yoy"`   // 净利同比 %
    EPS            float64 `json:"eps"`              // 每股收益（元）
    ROE            float64 `json:"roe"`              // 加权净资产收益率 %
    GrossMargin    float64 `json:"gross_margin"`     // 销售毛利率 %
    NetMargin      float64 `json:"net_margin"`       // 销售净利率 %
    DebtRatio      float64 `json:"debt_ratio"`       // 资产负债率 %
    CashFlowToRev  float64 `json:"cash_flow_to_rev"` // 经营现金流/营收
    DeductedProfit float64 `json:"deducted_profit"`  // 扣非净利润（亿元）
}

// FinanceResult 财务解析结果
type FinanceResult struct {
    Code       string                `json:"code"`
    Name       string                `json:"name"`
    Indicators []FinancialIndicator  `json:"indicators"` // 5 年年报 + 最新报告期
    Degraded   bool                  `json:"degraded"`
    ErrMsg     string                `json:"err_msg,omitempty"`
}
```

## 4. API 设计

### GET /api/finance?code=600519&market=沪A
- 响应 200：`FinanceResult`（indicators 按报告期降序）
- 失败 502（双源都挂，透传摘要）；参数非法 400

### POST /api/finance/analyze（SSE）
- body：`{"code":"600519","market":"沪A"}`
- 流程：服务端拉指标 → 格式化注入 LLM → 流式 delta → done / error
- **计入会话配额**（耗 LLM；与 /api/ask、/api/chat 共享 20 次/页）
- 事件协议复用：`delta`×N → `done` / `error`

## 5. 时序图

```plantuml
@startuml
Browser -> server: GET /api/finance?code=600519
server -> internal/finance: GetResult(ctx, code, market)
internal/finance -> 东财datacenter: RPT_F10_FINANCE_MAINFINADATA (REPORT_TYPE=年报, 6期)
internal/finance -> 东财F10: ZYZBAjaxNew（兜底/补名称）
internal/finance --> server: FinanceResult（5年报+最新期）
server --> Browser: 200 JSON（前端渲染表格）
Browser -> server: POST /api/finance/analyze（配额扣减）
server -> internal/agent: AnalyzeFinance(ctx, name, indicators, sink)
internal/agent -> deepseek: 财务解析 prompt + 指标文本
deepseek --> internal/agent: 流式
internal/agent -> compliance: Filter(delta)
internal/agent --> server: delta 事件
server --> Browser: SSE delta...done
@enduml
```

## 6. 详细设计

### 6.1 finance dao（internal/finance/finance.go）
- **主接口**：东财 datacenter `RPT_F10_FINANCE_MAINFINADATA`，`filter=(SECUCODE="600519.SH")(REPORT_TYPE="年报")`，`sortColumns=REPORT_DATE` 降序，`pageSize=6` → 最近 5 年年报 + 自动取**最新报告期**（同接口不带年报过滤取第 1 条）
- **兜底接口**：东财 F10 `ZYZBAjaxNew?type=0&code=SH600519`（页面接口，实测可用），解析首屏报告期，过滤 12-31 年报 + 最新一期；数量不足则少显示并置 `Degraded`
- **市场前缀**：`market`（沪A/深A/北交）→ `SH/SZ/BJ`；缺失时按代码首位推断（6/9→SH，0/3→SZ，4/8→BJ）
- **归一化**：金额字段 ÷1e8 → 亿元（保留 2 位小数）；百分比原值保留 2 位；空值 → 0
- 客户端 `http.Client{Timeout: 6s}` + 浏览器 UA；**串行**取主备（实测并发触发东财风控返回 code 9201 空数据）
- **代码格式修正（实测）**：datacenter 的 `SECUCODE` 需 `代码.交易所`（`600519.SH`），F10 需 `交易所+代码`（`SH600519`）——两个接口分开拼接，避免 9201「返回数据为空」

### 6.2 AI 解析（internal/agent/finance.go）
- 复用 `newDeepSeekModel` + `compliance.Filter/FilterFinal`（与 chat.go 相同流式模式）
- prompt：财务分析助手骨架（见设计方案 §5.2），事实注入格式：
  ```
  公司：贵州茅台（600519）
  最近 5 年年报 + 最新报告期（数据单位：亿元 / %）：
  | 报告期 | 营收 | 营收同比 | 净利 | 净利同比 | 每股收益 | ROE | 毛利率 | 净利率 | 负债率 | 经营现金流/营收 |
  | 2025年报 | 1721.0 | +? | ... |
  ...
  ```
- 输出 5 段：盈利质量 / 成长性 / 财务健康 / 值得关注的指标（同比波动>20% 强调）/ 风险提示；文末「数据来源：东方财富 · 不构成投资建议」

### 6.3 前端（财务解析卡片）
- 位置：详情页「情绪面板」卡片**上方**（用户确认调整）
- 结构：标题「财务解析」+ 数据源标注「东方财富 · 5年年报」+ 指标表格（行=指标，列=报告期）+ AI 解析区
- `finance.js`：`loadFinance(code, market)` 在 stock 事件后调用（与 fetchKline/fetchNews 并行）→ 渲染表格 → **自动** `POST /api/finance/analyze`（readSSE 流式渲染）
- 失败：卡片显示「财务数据暂不可用 + 重试」（不阻塞其他功能）；401/403 配额提示复用

## 7. 文件改动清单

| 文件 | 动作 | 说明 |
|---|---|---|
| internal/types/types.go | 改 | FinancialIndicator / FinanceResult |
| internal/finance/finance.go | 新增 | 东财 datacenter+F10 客户端、归一化、market 前缀 |
| internal/agent/finance.go | 新增 | 财务解析 prompt + 流式 + 合规 |
| internal/server/server.go | 改 | FinanceProvider 接口 + New 第 12 参 + 路由 |
| internal/server/handler_finance.go | 新增 | GET /api/finance + POST /api/finance/analyze |
| cmd/server/main.go | 改 | 组装 financeProvider |
| web/index.html | 改 | 财务解析卡片结构 |
| web/js/finance.js | 新增 | 表格渲染 + AI 解析流式 |
| web/js/app.js | 改 | stock 事件后 loadFinance |
| web/css/style.css | 改 | 财务卡片样式 |
| features/finance/SDD.md | 本文件 | 技术设计 |
| docs/CHANGELOG.md | 改 | 里程碑 |

## 8. 验证方案

1. 单测：market 前缀推断、datacenter/F10 JSON 解析、亿元归一化、双源兜底（主接口 mock 失败走备）
2. 接口冒烟：`/api/finance?code=600519` 返回 5 年年报真实数据（对照东财页面）；analyze SSE 流式
3. 合规：诱导「财报好是不是该买」→ 拒绝 + 风险提示；输出无概率词
4. 配额：/api/finance 不计数，/api/finance/analyze 计数
5. 回归：/api/ask、/api/chat、密钥弹窗、媒体播放不受影响；`go build/vet/test` 全绿

## 9. 上线三板斧

1. **灰度**：本机真实密钥全链路（600519 财报表格 + AI 解析）→ 服务器部署验证
2. **监控**：`[finance]` 日志（主/备源、耗时、指标条数）；AI 解析错误统计
3. **回滚**：删除 `/api/finance`、`/api/finance/analyze` 路由即下线功能（前后端独立，一期/二期不受影响）
