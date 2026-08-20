package server

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"
)

// handleKline GET /api/kline?code=&market=&days=：报价 + 日K线（展示数据，不进 LLM）
func (s *Server) handleKline(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code is required"})
		return
	}
	market := r.URL.Query().Get("market")
	if market == "" {
		market = "沪A"
	}
	days := 60
	if d := r.URL.Query().Get("days"); d != "" {
		n, err := strconv.Atoi(d)
		if err != nil || n < 10 || n > 250 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "days 需为 10-250 的整数"})
			return
		}
		days = n
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	start := time.Now()
	kl, err := s.klineProvider.GetKline(ctx, market, code, days)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		log.Printf("[kline] code=%s market=%s 失败: %v 耗时=%dms", code, market, err, elapsed)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "数据获取失败，请稍后重试"})
		return
	}
	log.Printf("[kline] code=%s market=%s candles=%d 耗时=%dms", code, market, len(kl.Candles), elapsed)
	writeJSON(w, http.StatusOK, kl)
}
