# 首页热门推荐 + 看山吉祥物主题 — 软件设计文档（SDD）

> 版本：v0.2 · 日期：2026-08-16 · 分支：`ui_premiu`
> 依据：`features/ui_premiu/设计方案.md`（产品/UX 设计）、`features/ui_premiu/SRS.md`（需求）

---

## 1. 概述

为首页新增「热门板块 + 热门股票」推荐（ticker 行情条 + 面板卡片），并接入看山吉祥物素材。数据源为东财涨幅榜（免登录公开接口），**展示数据不进 LLM**（合规，与 K线/资讯同策略）。

## 2. 总体架构（分层归属）

```
internal/hot（dao 层，新增）—— 东财热门客户端
internal/server（handler_hot.go + HotProvider 接口）—— GET /api/hot
web/pic/（素材）—— 看山 PNG/JPG（go:embed 内嵌）
web/css/style.css —— ticker / 热门卡片 / 吉祥物样式
web/js/{api,ui,app}.js —— apiHot / 渲染 / 首页加载与点击查询
```

新增依赖注入链（`Server.New` 6 参）：
```
cmd/server.main → hotProviderFunc(hot.GetHot) → server.HotProvider
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

响应 200：`[HotItem, ...]`；失败 502（透传摘要）。

## 5. 时序图

```plantuml
@startuml
Browser -> server: 首页 load → GET /api/hot?type=stock / type=sector（并行）
server -> internal/hot: GetHot(ctx, typ, count)
internal/hot -> eastmoney: clist/get（免登录，浏览器 UA）
eastmoney --> internal/hot: data.diff[]
internal/hot --> server: []HotItem
server --> Browser: 200 JSON ×2
Browser -> Browser: renderTicker + renderHotStocks + renderHotSectors
alt 用户点击热门股票
    Browser -> Browser: 输入框赋值 + doSearch()（走原有 SSE 链路）
end
@enduml
```

## 6. 模块与方法说明

### internal/hot（dao 层）

**`func GetHot(ctx context.Context, typ string, count int) ([]types.HotItem, error)`**
- 边界：typ 非 stock/sector → 明确错误；count clamp [1,20]；东财非 200/解析失败 → 明确错误；空结果 → 空数组
- 实现：按 typ 选 fs 参数（stock=沪深A股 / sector=行业板块 `m:90+t:2`）；字段映射 f12→Code、f14→Name、f2→Price、f3→ChangePct；请求带浏览器 UA（同 kline/news 模式）

### internal/server

**`type HotProvider interface { GetHot(ctx, typ string, count int) ([]types.HotItem, error) }`**
- 消费方定义；`cmd/server` 用 `hotProviderFunc` 适配器注入

**`func (s *Server) handleHot(w, r)`**：type 校验（400）、count clamp、调 provider、200/400/502、结构化日志

### cmd/server/main.go

- `hotProviderFunc(hot.GetHot)` + 编译期断言 `_ server.HotProvider`

### 前端

| 文件 | 职责 |
|---|---|
| `web/js/api.js` | `apiHot(type, count)`（新增） |
| `web/js/ui.js` | `renderTicker` / `renderHotStocks` / `renderHotSectors`（新增，红涨绿跌） |
| `web/js/app.js` | `loadHomeHot()`（init 并行拉 stock+sector）+ `hotSearch(name)`（赋值+查询） |
| `web/css/style.css` | `.ticker-bar/.ticker-item`、`.hot-chips/.hot-chip`、`.hot-stocks/.hot-stock`、`.mascot` |
| `web/index.html` | ticker 条 + 热门板块/股票卡片（搜索视图内）+ hero 吉祥物 `<img>` |
| `web/embed.go` | `//go:embed index.html css js pic`（新增 pic） |

## 7. 素材管线

- 源素材：`pic/看山三视图/`（JPG）、`pic/刘看山3d+平面/`（PNG）
- 拷贝至 `web/pic/`：`kanshan.png`（四视图，hero 用）、`kanshan-hero.jpg`（备用）
- `.gitignore` 排除大源文件（`*.obj *.c4d *.ai *.psd`），仓库只保留小图

## 8. 边界处理

| 场景 | 处理 |
|---|---|
| 热门接口失败 | 前端静默隐藏 ticker/卡片，搜索不受影响 |
| 空结果 | 隐藏对应卡片 |
| 板块点击 | 当前仅展示（成分股查询 P1 后置） |
| 明暗主题 | 新组件全用 CSS 变量，自动适配 |

## 9. 上线三板斧（demo）

- 监控：`[hot] type=stock count=8 耗时=XXms` 结构化日志
- 灰度：渐进增强（失败即隐藏），天然可回退
- 回滚：独立接口与卡片，可单独下线

## 10. 实现拆分（已实现 ✅）

1. ✅ `internal/hot` + `/api/hot` + 接线
2. ✅ 素材入 `web/pic` + embed
3. ✅ 首页重构（ticker + 热门卡片 + hero 吉祥物）
4. ✅ JS（apiHot / 渲染 / loadHomeHot / hotSearch）
5. ⏳ 待确认：板块点击行为（成分股）、ticker 自动滚动动画

## 11. 待确认

- [x] 板块 chip 点击 → 查询板块内成分股（已实现，腾讯 board_code 接口）
- [x] ticker 自动滚动（CSS 无缝循环 + 悬停暂停 + reduced-motion 适配）
