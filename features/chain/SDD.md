# AI 生成产业链流程图（可交互）— 软件设计文档（SDD）

> 版本：v0.1 · 日期：2026-08-22 · 分支：`feature/chain`（基于 main）
> 依据：`features/chain/设计方案.md`（v0.1）
> 决策：左右三列流程图 · 点击节点看同类型厂商 + 可跳查 · AI 动态生成（服务端校验厂商代码）

---

## 1. 概述

详情页新增「产业链图谱」卡片：AI 生成股票所在产业链（上游→中游→下游三列节点 + 连线），前端 SVG 渲染可交互——点击节点展开同类型厂商列表，点击厂商跳查基本面。

## 2. 总体架构

```
internal/agent/chain.go（新增）—— LLM 生成产业链 JSON（严格 JSON 约束 + 重试 + 解析）
internal/chain（service，新增）—— 组装：resolve → agent 生成 → 厂商代码校验（stock.Resolve）
internal/server/handler_chain.go（新增）—— POST /api/chain
web/index.html + web/js/chain.js + css —— 产业链卡片 + SVG 三列渲染 + 节点交互
```

注入链：`cmd/server.main → chainProvider{svc: chain.Service} → server.ChainProvider`（`server.New` 参数）

## 3. 数据结构（internal/types 新增）

```go
// ChainNode 产业链节点（环节）
type ChainNode struct {
    ID    string `json:"id"`              // n1, n2...
    Name  string `json:"name"`            // 环节名，如 上游-粮食/包装
    Stage string `json:"stage"`           // 上游/中游/下游
    Desc  string `json:"desc"`            // 环节说明 ≤20 字
}
// ChainEdge 环节间上下游关系
type ChainEdge struct {
    From string `json:"from"`
    To   string `json:"to"`
}
// ChainCompany 环节内同类型厂商
type ChainCompany struct {
    Code string `json:"code"` // A 股代码（已校验）
    Name string `json:"name"`
}
// ChainResult 产业链图谱
type ChainResult struct {
    Industry  string                    `json:"industry"`  // 产业链名，如 白酒产业链
    Nodes     []ChainNode               `json:"nodes"`
    Edges     []ChainEdge               `json:"edges"`
    Companies map[string][]ChainCompany `json:"companies"` // nodeID → 厂商（3-6 家）
    Degraded  bool                      `json:"degraded"`
    ErrMsg    string                    `json:"err_msg,omitempty"`
}
```

## 4. API 设计

### POST /api/chain（计配额）
- body `{code, market}`；返回 200 `ChainResult`
- 流程：resolve 股票 → agent 生成（**失败重试 1 次**）→ 厂商代码校验 → 返回
- 错误：LLM 两次失败 → 400「产业链生成失败」；其他 502
- **计配额**（与 ask/chat/finance-analyze/emotion-analyze 共享）

## 5. 详细设计

### 5.1 AI 生成（internal/agent/chain.go）
- 复用 `newDeepSeekModel` + 单次 `Invoke`（非流式，JSON 完整性优先）
- prompt（严格 JSON 约束，见设计方案 §3.1）：环节 6-10、每环节 3-6 家真实 A 股厂商、只输出 JSON
- 解析：`json.Unmarshal` → 失败重试 1 次（新 prompt 强调 JSON）；两次失败返回 error
- 结构校验：nodes 非空、stage 合法（上游/中游/下游）、edges 端点存在于 nodes、companies 键对应 nodeID——非法项过滤，关键字段缺失降级

### 5.2 厂商校验（internal/chain/chain.go）
- 对 companies 中每个 code 调 `stock.Resolve`（复用，验证存在性 + 拿规范名）
- 无效代码移除；该环节全无效 → 节点 companies 空 + 标注
- 校验全部失败不阻塞（`Degraded=true` + ErrMsg）

### 5.3 handler_chain（internal/server/handler_chain.go）
- 参数校验（code 非空）→ 配额 → ChainProvider → 200/400/502
- 复用 writeJSON；`Timeout 30s`（AI 生成）

### 5.4 前端（web/js/chain.js + SVG）
- 详情页 `#chain-card`（基本面卡之后）：「产业链图谱 · AI 生成仅供参考」
- 加载：`doSearch` 的 stock 事件后 `loadChain(code, market)`（POST /api/chain）
- **三列布局**：上游/中游/下游三列；每列节点垂直排列；按 edges 画连线（SVG polyline + 箭头 marker）
  - 列坐标：三列 x 中心（W/6, W/2, 5W/6）；节点 y 按列内序号均分
  - 连线：`<path>` 贝塞尔曲线连接 from→to（跨列），箭头 marker
- **节点**：圆角矩形（名称 + stage 标签 + desc），hover 高亮；列标题（上游/中游/下游）
- **交互**：点击节点 → 下方「同类型厂商」面板（该环节 companies：代码+名称+「查看」按钮）；点「查看」→ `$('stock-input').value = code; doSearch()`
- 节点过多：横向滚动容器（min-width）+ 缩放按钮（±，transform: scale）
- 失败：卡片「产业链生成失败 + 重试」

### 5.5 合规
- 「AI 生成 · 仅供参考」标注；厂商跳查走既有搜索链路（基本面/情绪合规）

## 6. 时序图

```plantuml
@startuml
Browser -> server: POST /api/chain {code, market}（计配额）
server -> internal/chain: 组装
internal/chain -> stock.Resolve: 识别股票名
internal/chain -> agent.GenerateChain: LLM 生成产业链 JSON（失败重试1次）
internal/chain -> stock.Resolve: 校验各环节厂商代码（无效过滤）
internal/chain --> server: ChainResult
server --> Browser: 200 JSON
Browser -> chain.js: 渲染三列 SVG（节点+连线）
Browser -> chain.js: 点击节点 → 同类型厂商面板 → 点厂商 → doSearch(code)
@enduml
```

## 7. 文件改动清单

| 文件 | 动作 |
|---|---|
| internal/types/types.go | 改：ChainNode/ChainEdge/ChainCompany/ChainResult |
| internal/agent/chain.go + _test.go | 新增：LLM 生成 + JSON 解析/重试 |
| internal/chain/chain.go + _test.go | 新增：组装 + 厂商校验 |
| internal/server/handler_chain.go | 新增：POST /api/chain |
| internal/server/server.go | 改：ChainProvider 接口 + 路由 + New 参数 |
| cmd/server/main.go | 改：组装 chainProvider |
| web/index.html | 改：产业链卡片 |
| web/js/chain.js | 新增：三列 SVG 渲染 + 节点交互 |
| web/js/app.js | 改：stock 事件后 loadChain |
| web/css/style.css | 改：产业链卡片/图样式 |
| features/chain/SDD.md | 本文件 |
| docs/CHANGELOG.md | 改 |

## 8. 验证方案

1. 单测：chain JSON 解析（合法/非法/重试）、stage 校验、厂商过滤（有效/无效）
2. 接口冒烟：茅台 → 白酒产业链（三列节点 + 厂商可 Resolve）；非法 code 400
3. 前端：三列渲染、连线、节点点击厂商面板、厂商跳查
4. 合规：无荐股；「AI 生成仅供参考」标注
5. 回归：基本面/情绪/对话/密钥不受影响；`go build/vet/test`

## 9. 上线三板斧

1. **灰度**：本机茅台/宁德真实产业链（对照行业常识检查环节与厂商）
2. **监控**：`[chain]` 日志（生成耗时、重试次数、厂商校验过滤数、降级原因）
3. **回滚**：删除 `/api/chain` 路由 + 前端卡片即下线（前后端独立）
