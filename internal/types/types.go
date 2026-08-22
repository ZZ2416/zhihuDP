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

// Event 对外事件（SSE 事件协议：stock / fundamental / delta / done / error）
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
	Finance      string // 财务指标摘要（最近5年年报+最新期）
	Valuation    string // 估值摘要（PE/PB/分位，功能点7注入）
	Score        string // 四维评分摘要（功能点7注入）
	PrevAnalysis string // 一期 AI 分析文本
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

// VideoItem 视频资讯（B站，GET /api/video 返回体元素）
type VideoItem struct {
	Title       string `json:"title"` // 已清洗高亮标签
	Url         string `json:"url"`   // https://www.bilibili.com/video/BVxxx
	Pic         string `json:"pic"`   // 封面图 URL（https://）
	Bvid        string `json:"bvid"`
	Play        int64  `json:"play"`         // 播放量
	Danmaku     int64  `json:"danmaku"`      // 弹幕数
	Duration    string `json:"duration"`     // mm:ss
	PublishTime string `json:"publish_time"` // YYYY-MM-DD HH:MM
	Author      string `json:"author"`       // UP主
	Degraded    bool   `json:"degraded"`
}

// Valuation 估值（当前值 + 历史分位）
type Valuation struct {
	PE           float64 `json:"pe"`             // PE(TTM)，统一东财口径
	PB           float64 `json:"pb"`             // PB(MRQ)，腾讯
	MarketCap    float64 `json:"market_cap"`     // 总市值（亿元），腾讯
	PEEntPercent float64 `json:"pe_ent_percent"` // 当前 PE 历史分位 0-100；无数据 -1
	Degraded     bool    `json:"degraded"`
}

// FundamentalScore 四维基本面评分（0-100）
type FundamentalScore struct {
	Profit int      `json:"profit"`            // 盈利能力
	Growth int      `json:"growth"`            // 成长性
	Health int      `json:"health"`            // 财务健康
	Valuat int      `json:"valuation"`         // 估值
	Total  int      `json:"total"`             // 加权总分
	Grade  string   `json:"grade"`             // 质地强/良好/一般/偏弱
	NoData []string `json:"no_data,omitempty"` // 数据不足的维度
}

// FundamentalResult 基本面聚合结果（指标 + 估值 + 评分）
type FundamentalResult struct {
	Code       string               `json:"code"`
	Name       string               `json:"name"`
	Indicators []FinancialIndicator `json:"indicators"`
	Valuation  Valuation            `json:"valuation"`
	Score      FundamentalScore     `json:"score"`
	Degraded   bool                 `json:"degraded"`
}

// ChainNode 产业链节点（环节）
type ChainNode struct {
	ID    string `json:"id"`    // n1, n2...
	Name  string `json:"name"`  // 环节名，如 上游-粮食/包装
	Stage string `json:"stage"` // 上游/中游/下游
	Desc  string `json:"desc"`  // 环节说明 ≤20 字
}

// ChainEdge 环节间上下游关系
type ChainEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// ChainCompany 环节内同类型厂商（已校验 A 股代码）
type ChainCompany struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// ChainResult 产业链图谱（AI 生成，服务端校验）
type ChainResult struct {
	Industry  string                    `json:"industry"` // 产业链名
	Nodes     []ChainNode               `json:"nodes"`
	Edges     []ChainEdge               `json:"edges"`
	Companies map[string][]ChainCompany `json:"companies"` // nodeID → 厂商
	Degraded  bool                      `json:"degraded"`
	ErrMsg    string                    `json:"err_msg,omitempty"`
}
