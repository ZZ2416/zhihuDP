// Package types 共享数据结构与哨兵错误
package types

import "errors"

// 哨兵错误（跨包边界使用，避免 handler 反向依赖 data 层）
var (
	ErrEmptyQuery    = errors.New("empty query")
	ErrStockNotFound = errors.New("stock not found")
)

// StockInfo 股票识别结果
type StockInfo struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Market string `json:"market"` // 如 沪A / 深A / 北A
}

// Ratio 多空占比（0-1 比例）
type Ratio struct {
	Bull    float64 `json:"bull"`
	Bear    float64 `json:"bear"`
	Neutral float64 `json:"neutral"`
}

// ViewItem 代表观点
type ViewItem struct {
	Title     string `json:"title"`
	Url       string `json:"url"`
	Author    string `json:"author"`
	VoteUp    int    `json:"vote_up"`
	Excerpt   string `json:"excerpt"`
	Sentiment string `json:"sentiment"` // bull/bear/neutral
}

// SentimentResult 情绪面板结构化数据（SSE sentiment 事件负载）
type SentimentResult struct {
	Code     string     `json:"code"`
	Name     string     `json:"name"`
	Heat     int        `json:"heat"`   // 讨论量（demo：本次取回条数）
	Sample   int        `json:"sample"` // 实际分类样本数
	Ratio    Ratio      `json:"ratio"`
	Score    *int       `json:"score"`    // 参考强度 1-10；样本不足为 nil
	Items    []ViewItem `json:"items"`    // 代表观点 ≤5
	Degraded bool       `json:"degraded"` // true=降级（样本不足/搜索失败）
	ErrMsg   string     `json:"err_msg,omitempty"`
}

// Event 对外事件（SSE 事件协议：stock / sentiment / delta / done / error）
type Event struct {
	Type string
	Data any
}

// Candle 单根日K
type Candle struct {
	Date   string  `json:"date"` // 2026-08-12
	Open   float64 `json:"open"`
	Close  float64 `json:"close"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Volume float64 `json:"volume"` // 手
}

// Quote 实时报价（东财字段 ÷100 换算后）
type Quote struct {
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	Price     float64 `json:"price"`      // 最新价
	Change    float64 `json:"change"`     // 涨跌额
	ChangePct float64 `json:"change_pct"` // 涨跌幅 %
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Volume    float64 `json:"volume"` // 手
	PrevClose float64 `json:"prev_close"`
}

// Kline 行情响应（GET /api/kline 返回体）
type Kline struct {
	Quote   Quote    `json:"quote"`
	Candles []Candle `json:"candles"` // 升序（旧→新）；空数组表示无数据
}

// NewsItem 相关资讯（GET /api/news 返回体元素）
type NewsItem struct {
	Title  string `json:"title"`  // 已清洗 <em> 高亮标签
	Url    string `json:"url"`
	Date   string `json:"date"`   // 2026-08-14 23:18:00
	Source string `json:"source"` // 固定 "东方财富"
}

// HotItem 热门榜单项（股票或板块，GET /api/hot 返回体元素）
type HotItem struct {
	Code      string  `json:"code"`       // 股票代码 / 板块代码
	Name      string  `json:"name"`
	Price     float64 `json:"price"`      // 最新价（板块为指数点位）
	ChangePct float64 `json:"change_pct"` // 涨跌幅 %
	Type      string  `json:"type"`       // stock / sector
}


// KnowledgeItem 知识库搜索结果条目（GET /api/knowledge 返回体元素）
type KnowledgeItem struct {
	Content  []string `json:"content"`  // 命中的内容片段
	DocName  string   `json:"doc_name"` // 文档名（讨论标题）
	OriginUrl string  `json:"origin_url"` // 知乎原文链接
}
