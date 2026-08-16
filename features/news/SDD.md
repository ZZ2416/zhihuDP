# 相关资讯功能 — 设计文档（SDD）

> 版本：v0.1 · 日期：2026-08-16 · 分支：`feature/news`
> 依据：`features/news/SRS.md`
> 模式：与 K线功能完全一致（展示数据不进 LLM，前端异步拉取）

## 1. 分层归属

```
internal/news（dao 层，新增）—— 东财资讯搜索客户端
internal/server（handler_news.go + NewsProvider 接口）—— GET /api/news
web/js/news.js（或并入 api/ui）—— 前端资讯卡片
web/css/style.css —— 卡片样式
```

## 2. 数据流

```
前端收到 stock 事件（name）→（异步）GET /api/news?keyword=name&count=5
  → internal/news.GetNews → 东财搜索接口 → JSON
  → 前端渲染「相关资讯」卡片（失败静默隐藏）
```

## 3. 数据结构（internal/types 新增）

```go
type NewsItem struct {
    Title  string `json:"title"`  // 已清洗 <em> 标签
    Url    string `json:"url"`
    Date   string `json:"date"`   // 2026-08-14 23:18:00
    Source string `json:"source"` // 固定 "东方财富"
}
```

## 4. API 设计

### GET /api/news

| 参数 | 类型 | 必填 | 说明 | 边界 |
|---|---|---|---|---|
| keyword | string | 是 | 股票名称 | 空 → 400 |
| count | int | 否 | 条数 | clamp [1,10]，默认 5 |

响应 200：`[{title,url,date,source}, ...]`；失败 502（透传摘要）。

## 5. 时序图

```plantuml
@startuml
Browser -> server: GET /api/news?keyword=贵州茅台&count=5
server -> internal/news: GetNews(ctx, keyword, count)
internal/news -> eastmoney: search/jsonp（免登录）
eastmoney --> internal/news: cmsArticleWebOld[]
internal/news --> server: []NewsItem（title 清洗 <em>）
server --> Browser: 200 JSON
Browser -> Browser: 渲染「相关资讯」卡片
alt 失败
    Browser -> Browser: 隐藏卡片（静默降级）
end
@enduml
```

## 6. 方法说明

### internal/news

**`func GetNews(ctx context.Context, keyword string, count int) ([]types.NewsItem, error)`**
- 边界：keyword 空 → 错误；count clamp [1,10]；东财非 200/JSONP 解析失败 → 明确错误；空结果 → 空数组
- 实现：URL 构造 param JSON（json.Marshal 后 QueryEscape），请求带浏览器 UA；解析 JSONP（剥 `cb(` 与 `)`）；title 用正则去 `<em></em>`

### internal/server

- `NewsProvider` 接口 + `Server.New` 增参（analyzer, resolver, klineProvider, newsProvider, frontend）
- `handleNews`：参数校验（keyword 必填、count clamp）→ 调 provider → 200/400/502

### 前端

- `web/js/app.js`：stock 事件 → `fetchNews(d.name)`
- 资讯卡片放 AI 分析之后（辅助信息置后）；样式 `.news-item`（标题链接 + 时间，hover 主题色）

## 7. 上线三板斧（demo）

- 监控：`[news] keyword=... count=5 耗时=XXms` 结构化日志
- 灰度：前端渐进增强，失败即隐藏卡片，天然可回退
- 回滚：独立接口与卡片，可单独下线

## 8. 待确认

- [ ] 资讯卡片位置（AI 分析之后 / 情绪面板之前）——默认 AI 分析之后
- [ ] 关键词用股票全名（贵州茅台）还是简称（茅台）——默认全名（东财搜索更准）
