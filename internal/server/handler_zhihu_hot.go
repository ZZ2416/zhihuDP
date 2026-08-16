package server

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"zhihudp/internal/types"
)

// financeKeywords 金融相关关键词（标题+摘要命中即保留；知乎热榜为全站热榜，过滤掉与金融无关的条目）
var financeKeywords = []string{
	"股票", "股市", "A股", "港股", "美股", "基金", "银行", "券商", "保险", "债券",
	"黄金", "外汇", "汇率", "房地产", "楼市", "房价", "买房", "购房", "贷款", "理财",
	"投资", "上市", "财报", "业绩", "盈利", "股价", "牛市", "熊市", "央行", "利率",
	"降息", "加息", "通胀", "通货膨胀", "GDP", "经济", "贸易", "关税", "人民币", "美元",
	"市值", "融资", "IPO", "花呗", "泡沫", "公司", "企业", "市场", "涨价", "降价",
}

// isFinance 判断热榜条目是否与金融相关
func isFinance(it types.ZhihuHotItem) bool {
	text := it.Title + " " + it.Summary
	for _, kw := range financeKeywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

// handleZhihuHot GET /api/zhihu-hot?count=：知乎热榜（仅金融相关，主数据源，每 3h 更新）
func (s *Server) handleZhihuHot(w http.ResponseWriter, r *http.Request) {
	count := 10
	if c := r.URL.Query().Get("count"); c != "" {
		n, err := strconv.Atoi(c)
		if err != nil || n < 1 || n > 30 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "count 需为 1-30 的整数"})
			return
		}
		count = n
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	start := time.Now()
	all, err := s.zhihuHot.HotList(ctx, 30) // 拉全量再过滤
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		log.Printf("[zhihu-hot] 失败: %v 耗时=%dms", err, elapsed)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	// 金融相关过滤（下掉与金融无关的热榜）
	items := make([]types.ZhihuHotItem, 0, count)
	for _, it := range all {
		if isFinance(it) {
			items = append(items, it)
			if len(items) >= count {
				break
			}
		}
	}
	log.Printf("[zhihu-hot] 金融相关 %d/%d 条 耗时=%dms", len(items), len(all), elapsed)
	writeJSON(w, http.StatusOK, items)
}
