# 相关资讯功能 — 需求规格（SRS）

> 版本：v0.1 · 日期：2026-08-16 · 分支：`feature/news`
> 定位：**辅助数据源**（不替代知乎情绪，补充事件面信息）

## 1. 背景与目标

搜索股票后，用户除了「行情 + 情绪 + AI 分析」，还需要了解**近期相关新闻/资讯**（业绩、公告、行业事件），辅助理解讨论背景。

**数据源**：东方财富资讯搜索接口（免登录、公开，已实测 2026-08-16），零合规风险。

## 2. 功能需求

| # | 需求 | 优先级 | 验收 |
|---|---|---|---|
| FR-N1 | 股票详情页展示「相关资讯」卡片（5 条：标题 + 时间 + 来源） | P0 | 搜「茅台」显示 5 条真实资讯；**仅参考展示，不可点击跳转外部**（知乎以外数据只借鉴不外链） |
| FR-N2 | 资讯为**展示数据，不进 LLM 上下文**（与 K线同策略，防模型基于新闻给买卖建议） | P0（合规） | 代码评审：agent/sentiment 无 news 依赖 |
| FR-N3 | 资讯加载失败**静默降级**（隐藏卡片，不影响其余展示） | P1 | 断网时情绪/AI 正常 |

## 3. 数据契约（东财搜索资讯，已实测）

```
GET https://search-api-web.eastmoney.com/search/jsonp?cb=cb&param={json}
param = {"uid":"","keyword":"{股票名}","type":["cmsArticleWebOld"],"client":"web",
         "clientType":"web","clientVersion":"curr",
         "param":{"cmsArticleWebOld":{"searchScope":"default","sort":"default",
           "pageIndex":1,"pageSize":{count},"preTag":"<em>","postTag":"</em>"}}}
```
- 返回 JSONP：`cb({...})`，正文 `result.cmsArticleWebOld[]`，字段：`title`（含 `<em>` 高亮标签，需清洗）、`date`、`url`

## 4. 验收

| # | 项 | 标准 |
|---|---|---|
| 1 | 接口 | `/api/news?keyword=贵州茅台&count=5` 返回 5 条合法 JSON |
| 2 | 展示 | 前端资讯卡片渲染标题/时间/链接，明暗主题适配 |
| 3 | 降级 | 接口失败时卡片隐藏，情绪/AI 不受影响 |
| 4 | 合规 | agent/sentiment 无 news 引用 |
