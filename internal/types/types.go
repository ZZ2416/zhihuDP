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
	Title  string `json:"title"` // 已清洗 <em> 高亮标签
	Url    string `json:"url"`
	Date   string `json:"date"`   // 2026-08-14 23:18:00
	Source string `json:"source"` // 固定 "东方财富"
}

// HotItem 热门榜单项（股票或板块，GET /api/hot 返回体元素）
type HotItem struct {
	Code      string  `json:"code"` // 股票代码 / 板块代码
	Name      string  `json:"name"`
	Price     float64 `json:"price"`      // 最新价（板块为指数点位）
	ChangePct float64 `json:"change_pct"` // 涨跌幅 %
	Type      string  `json:"type"`       // stock / sector
}

// KnowledgeItem 知识库搜索结果条目（GET /api/knowledge 返回体元素）
type KnowledgeItem struct {
	Content   []string `json:"content"`    // 命中的内容片段
	DocName   string   `json:"doc_name"`   // 文档名（讨论标题）
	OriginUrl string   `json:"origin_url"` // 知乎原文链接
}

// ChatMessage 追问对话消息（会话历史）
type ChatMessage struct {
	Role    string `json:"role"` // user / assistant
	Content string `json:"content"`
}

// ChatFacts 对话上下文事实快照（组装后注入 LLM）
type ChatFacts struct {
	StockName    string // 股票名
	StockCode    string
	Market       string // 市场（沪A/深A）
	Quote        string // 行情快照文本（报价级）
	Sentiment    string // 情绪面板摘要文本
	Finance      string // 财务指标摘要（最近5年年报+最新期）
	Knowledge    string // 知识库检索片段
	PrevAnalysis string // 一期 AI 分析文本（中性文案）
}

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
	Code       string               `json:"code"`
	Name       string               `json:"name"`
	Indicators []FinancialIndicator `json:"indicators"` // 5 年年报 + 最新报告期
	Degraded   bool                 `json:"degraded"`
	ErrMsg     string               `json:"err_msg,omitempty"`
}

// MinutePoint 分时数据点（每 1 分钟）
type MinutePoint struct {
	Time     string  `json:"time"`      // HH:MM
	Price    float64 `json:"price"`     // 最新价
	AvgPrice float64 `json:"avg_price"` // 均价线（东财源；腾讯兜底为 0）
	Volume   float64 `json:"volume"`    // 成交量（手）
}

// MinuteResult 分时数据（当日）
type MinuteResult struct {
	Code     string        `json:"code"`
	Name     string        `json:"name"`
	PreClose float64       `json:"pre_close"` // 昨收（分时涨跌参照）
	Points   []MinutePoint `json:"points"`
	Degraded bool          `json:"degraded"`
}
