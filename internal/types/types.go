// Package types 共享数据结构
package types

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
	Heat     int        `json:"heat"`     // 讨论量（demo：本次取回条数）
	Sample   int        `json:"sample"`   // 实际分类样本数
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
