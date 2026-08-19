# B站相关视频 — 软件设计文档（SDD）

> 版本：v1.0（已实现）· 日期：2026-08-19

## 1. 概述

详情页「相关视频」卡片：B站 wbi 签名搜索股票相关视频，前端按时间/播放量排序，标题可跳转。

## 2. 架构

```
internal/video（dao，新增）—— B站 wbi 签名 + 搜索
internal/server（handler_video.go + VideoProvider 接口）—— GET /api/video
web/js/video.js（新增）—— 加载 + 排序切换 + 渲染
```

注入链（`server.New` 第 14 参）：`videoProvider{get: video.GetVideos} → server.VideoProvider`

## 3. 数据结构

```go
type VideoItem struct {
    Title       string `json:"title"`        // 已清洗
    Url         string `json:"url"`          // bilibili.com/video/BVxxx
    Bvid        string `json:"bvid"`
    Play        int64  `json:"play"`
    Danmaku     int64  `json:"danmaku"`
    Duration    string `json:"duration"`     // mm:ss
    PublishTime string `json:"publish_time"` // YYYY-MM-DD HH:MM
    Author      string `json:"author"`
    Degraded    bool   `json:"degraded"`
}
```

## 4. API

### GET /api/video?keyword=贵州茅台&count=8
- 200：`[]VideoItem`；keyword 必填；count clamp [1,20] 默认 10；失败 502

## 5. wbi 签名（B站公开算法）

```
1. GET /x/web-interface/nav → img_url/sub_url 文件名去扩展名 → img_key/sub_key
   （未登录 code=-101 不影响，wbi_img 仍返回）
2. mixinKey = 按 mixinKeyEncTab 重排 (img_key+sub_key) 取前 32 位
3. params + wts(时间戳) → 按 key 排序 → urlencode → md5(query + mixinKey) → w_rid
4. 缓存 mixinKey 2 小时
```

## 6. 前端

- `video.js`：`loadVideo(name)`（stock 事件后调用）→ `apiVideo(name, 15)` → `videoItems` 缓存
- `sortVideos(mode)`：「最新」按 `publish_time` 降序 /「播放量」按 `play` 降序（前端排序，一次拉取）
- 渲染：标题链接（新窗口）+ 播放量（万/亿格式化）+ 时长 + UP主 + 发布时间
- 切股：`doSearch` 重置卡片

## 7. 验证

- 单测：mixinKey（32 字符 key 固定值 + 短 key 防 panic）、标题清洗、时长（秒/mm:ss）、mock 搜索
- 接口冒烟：贵州茅台 8 条真实数据（时长 17:19/41:40 等、播放量、发布时间、BV 链接）
- 回归 + `go build/vet/test` 全绿
