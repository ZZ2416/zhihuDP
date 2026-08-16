package main

// StockInfo 股票识别结果
type StockInfo struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Market string `json:"market"` // 如 沪A / 深A / 北A
}

// --- M1 将新增的共享结构（占位） ---
//
// type Ratio struct {
//     Bull    float64 `json:"bull"`
//     Bear    float64 `json:"bear"`
//     Neutral float64 `json:"neutral"`
// }
//
// type ViewItem struct {
//     Title     string `json:"title"`
//     Url       string `json:"url"`
//     Author    string `json:"author"`
//     VoteUp    int    `json:"vote_up"`
//     Excerpt   string `json:"excerpt"`
//     Sentiment string `json:"sentiment"` // bull/bear/neutral
// }
//
// type SentimentResult struct {
//     Code     string     `json:"code"`
//     Name     string     `json:"name"`
//     Heat     int        `json:"heat"`
//     Sample   int        `json:"sample"`
//     Ratio    Ratio      `json:"ratio"`
//     Score    *int       `json:"score"`
//     Items    []ViewItem `json:"items"`
//     Degraded bool       `json:"degraded"`
//     ErrMsg   string     `json:"err_msg,omitempty"`
// }
//
// SSE 事件类型：stock / sentiment / delta / done / error（M2 定义）
