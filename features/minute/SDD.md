# 当日分时图 — 软件设计文档（SDD）

> 版本：v1.0（已实现）· 日期：2026-08-19 · 分支：`feature/minute`
> 依据：`features/minute/SRS.md`

---

## 1. 概述

详情页 K 线图上方新增「日K / 分时」切换，分时数据东财主 + 腾讯兜底，前端懒加载 + 缓存。

## 2. 总体架构

```
internal/minute（dao，新增）—— 东财 trends2 主 + 腾讯 minute 兜底
internal/server（handler_minute.go + MinuteProvider 接口）—— GET /api/minute
web/js/kline.js —— renderMinute（SVG 折线）+ switchKline（切换/缓存）
```

注入链（`server.New` 第 13 参）：
```
cmd/server.main → minuteProvider{get: minute.GetMinute} → server.MinuteProvider
```

## 3. 数据结构（internal/types 新增）

```go
type MinutePoint struct {
    Time     string  `json:"time"`      // HH:MM
    Price    float64 `json:"price"`     // 最新价
    AvgPrice float64 `json:"avg_price"` // 均价线（东财源；腾讯兜底累计均价）
    Volume   float64 `json:"volume"`    // 成交量（手）
}
type MinuteResult struct {
    Code     string        `json:"code"`
    Name     string        `json:"name"`
    PreClose float64       `json:"pre_close"` // 昨收
    Points   []MinutePoint `json:"points"`
    Degraded bool          `json:"degraded"`
}
```

## 4. API 设计

### GET /api/minute?code=600519&market=沪A
- 响应 200：`MinuteResult`（points 每 1 分钟）
- 失败 502（双源都挂）；参数非法 400；**展示数据，不计配额**

## 5. 时序图

```plantuml
@startuml
Browser -> server: 点「分时」→ GET /api/minute?code=600519
server -> internal/minute: GetMinute(ctx, market, code)
internal/minute -> 东财 trends2: secid=1.600519（主）
internal/minute -> 腾讯 minute: sh600519（兜底，主失败时）
internal/minute --> server: MinuteResult（points + preClose）
server --> Browser: 200 JSON
Browser -> kline.js: renderMinute（价格线+均价线+昨收虚线，涨红跌绿）
Browser -> Browser: 切回日K → 用 window.__lastKline 缓存重画（不重新请求）
@enduml
```

## 6. 详细设计

### 6.1 minute dao
- **东财主**：`trends2/get`，`secid = 市场.代码`（沪=1，深/北=0）；`trends[]` 格式 `时间,开,收,高,低,量,额,均价` → 取收价 + 均价
- **腾讯兜底**：`minute/query?code=sh600519`；`data[]` 格式 `HHMM 价格 量 额` → 累计均价 = 累计额/累计量/100
- 串行取主备（东财并发易触发风控，finance 同经验）
- `http.Client{Timeout: 6s}` + 浏览器 UA

### 6.2 前端交互（kline.js）
- **分时**：SVG 内置十字光标（`#mv-line/#mh-line/#m-hl`）+ HTML 明细浮层（复用 `.kline-tip`）；`el._m` 存几何上下文；`mousemove/touchstart → minuteHover`（十字光标 + 时间/价格/涨跌幅/均价/成交量），`click` 锁定，`mouseleave → minuteHide`
- **日K 切回修复**：切回日K 用缓存数据 `__lastKlineData` **重新调用 renderKline**（而非重填 innerHTML），恢复十字光标与事件绑定

### 6.3 前端切换（kline.js）
- 切换按钮：`#tab-day` / `#tab-minute`（`.kline-tabs`）
- `switchKline(mode)`：分时懒加载（`apiMinute` → `renderMinute`，结果缓存 `minuteCache`）；日K 用 `window.__lastKline` 缓存重画
- `renderMinute(data)`：SVG 折线——价格线（相对昨收红涨绿跌）+ 均价线（黄）+ 昨收虚线 + 右上角涨跌幅 + 底部时间刻度
- 切股重置：`doSearch` 清 `minuteCache` / `window.__lastKline` / 切回日K tab

## 6.4 数据结构备注
- `window.__lastKlineData = {candles, quote}`（日K 数据缓存，切回重新渲染）
- `window.minuteCache`（分时数据缓存，懒加载一次）
- `el._m`（分时交互几何上下文）/ `el._k`（日K 交互上下文）互不干扰

## 7. 文件改动清单

| 文件 | 动作 |
|---|---|
| internal/types/types.go | 改：MinutePoint / MinuteResult |
| internal/minute/minute.go + _test.go | 新增：双源 dao + 单测 |
| internal/server/handler_minute.go | 新增：GET /api/minute |
| internal/server/server.go | 改：MinuteProvider 接口 + New 第 13 参 + 路由 |
| cmd/server/main.go | 改：组装 minuteProvider |
| web/js/kline.js | 改：renderMinute + switchKline + 缓存 |
| web/js/api.js | 改：apiMinute |
| web/js/app.js | 改：切股重置分时 |
| web/index.html + css | 改：切换按钮 + 样式 |
| features/minute/SRS.md | 新增：需求规格 |

## 8. 验证

- 单测：secid 推断、东财 trends 解析、腾讯解析（累计均价）、兜底
- 接口冒烟：600519（东财）121 点 + 昨收 1297.99；000001 正常
- 回归：日K 渲染/十字光标/财报/情绪/AI 不受影响；`go build/vet/test` 全绿
