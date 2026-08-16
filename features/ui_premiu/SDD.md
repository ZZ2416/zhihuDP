# 首页热门推荐 + 看山主题 — 设计文档（SDD）

> 版本：v0.1 · 日期：2026-08-16 · 分支：`ui_premiu`
> 依据：`features/ui_premiu/SRS.md`

## 1. 分层归属

```
internal/hot（dao 层，新增）—— 东财热门（股票/板块）客户端
internal/server（handler_hot.go + HotProvider 接口）—— GET /api/hot
web/pic/（素材）—— 看山 PNG（go:embed 内嵌）
web/index.html / web/css/style.css / web/js/ —— 首页重构 + 终端风格
```

## 2. 数据流

```
首页加载 →（异步）GET /api/hot?type=stock&type=sector
  → internal/hot → 东财 clist（免登录）→ JSON
  → 前端渲染热门板块/股票卡片（失败静默隐藏）
点击热门股票 → 复用现有 doSearch（输入框赋值 + 查询）
```

## 3. 数据结构（internal/types 新增）

```go
// HotItem 热门榜单项（股票或板块）
type HotItem struct {
    Code      string  `json:"code"`       // 股票代码 / 板块代码
    Name      string  `json:"name"`
    Price     float64 `json:"price"`      // 最新价（板块为指数点位）
    ChangePct float64 `json:"change_pct"` // 涨跌幅 %
    Type      string  `json:"type"`       // stock / sector
}
```

## 4. API 设计

### GET /api/hot

| 参数 | 类型 | 必填 | 说明 | 边界 |
|---|---|---|---|---|
| type | string | 是 | stock / sector | 非法 → 400 |
| count | int | 否 | 条数 | clamp [1,20]，默认 stock=8 / sector=6 |

响应 200：`[HotItem, ...]`；失败 502。

## 5. 前端设计

### 首页结构（重构后）

```
hero（看山吉祥物大图 + 标题 + 副题）
search 卡片（不变）
热门板块 卡片（6 个板块 chip：名称 + 涨幅，红涨绿跌）
热门股票 卡片（8 行：名称 / 代码 / 最新价 / 涨幅；点击 → doSearch）
ticker 滚动条（可选，顶部复用热门股票数据，CSS 动画横向滚动）
```

### 终端风格（保留双主题）

- 顶部 **ticker**：热门股票横向滚动条（`overflow-x:auto` + 滚动动画，点击项也可查询）
- 详情区**网格化**：行情卡 / 情绪面板 / AI 分析 双栏排列（`@media` 断点回退单栏）
- 大字号排版：标题 `clamp(28px, 4vw, 48px)`，eyebrow 小标
- 色板：**保持现有明暗双主题 CSS 变量**（不引入第三主题），仅布局与交互对齐终端页

### 看山素材

- 复制 `pic/刘看山3d+平面/刘看山四视图.png` → `web/pic/kanshan.png`（约 16KB）
- `web/embed.go` 增加 `//go:embed pic`；hero 区展示（`border-radius` 圆角卡片 + 主题背景适配）

## 6. 方法说明

### internal/hot

**`func GetHot(ctx context.Context, typ string, count int) ([]types.HotItem, error)`**
- 边界：typ 非法 → 错误；count clamp；东财非 200/解析失败 → 明确错误；空结果 → 空数组
- 实现：按 typ 选 fs 参数（stock 沪深A / sector 行业板块），f12→Code, f14→Name, f2→Price, f3→ChangePct

### internal/server

- `HotProvider` 接口 + `Server.New` 增参（…newsProvider, hotProvider, frontend）
- `handleHot`：type 校验、count clamp、调 provider、200/400/502

### 前端

- `web/js/api.js`：`apiHot(type, count)`
- `web/js/ui.js`：`renderHotStocks(items)` / `renderHotSectors(items)`（红涨绿跌 chip）
- `web/js/app.js`：首页 `loadHomeHot()`（init 时并行拉 stock+sector）+ 点击热门股票 → 输入框赋值 + `doSearch()`

## 7. 上线三板斧

- 监控：`[hot] type=stock count=8 耗时=XXms`
- 灰度：渐进增强（热门卡片失败即隐藏，搜索不受影响）
- 回滚：独立接口与卡片，可单独下线

## 8. 待确认

- [ ] 板块点击行为：查询板块内个股（需板块成分接口，P1 后置）还是仅展示？
- [ ] ticker 是否要做点击查询（默认做）
