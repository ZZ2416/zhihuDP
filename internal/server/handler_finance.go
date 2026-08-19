// 财务解析 HTTP 处理器：GET /api/finance（指标数据）+ POST /api/finance/analyze（AI 解析 SSE）
package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"zhihudp/internal/types"
)

// financeRequest 财务请求参数
type financeRequest struct {
	Code   string `json:"code"`
	Market string `json:"market"`
}

// handleFinance GET /api/finance?code=600519&market=沪A
// 返回财务指标（展示数据，不计配额）。
func (s *Server) handleFinance(w http.ResponseWriter, r *http.Request) {
	if s.finance == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "财务服务未启用"})
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code 不能为空"})
		return
	}
	market := strings.TrimSpace(r.URL.Query().Get("market"))

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	start := time.Now()
	res, err := s.finance.GetFinance(ctx, code, market)
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		log.Printf("[finance] code=%s 失败: %v 耗时=%dms", code, err, elapsed)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	log.Printf("[finance] code=%s 指标=%d 耗时=%dms", code, len(res.Indicators), elapsed)
	writeJSON(w, http.StatusOK, res)
}

// handleFinanceAnalyze POST /api/finance/analyze
// 财报 AI 解析（SSE：delta → done / error）；耗 LLM，计入会话配额。
func (s *Server) handleFinanceAnalyze(w http.ResponseWriter, r *http.Request) {
	if s.finance == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "财务服务未启用"})
		return
	}
	var req financeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体必须是 JSON"})
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	if req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code 不能为空"})
		return
	}

	// 会话配额（与 /api/ask、/api/chat 共享）
	if _, ok := s.consumeQuota(w, r); !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "本次访问的 API 调用次数已用完（20 次），请重新打开页面"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "SSE 不受支持"})
		return
	}
	sink := func(ev types.Event) error {
		if err := writeSSE(w, ev.Type, ev.Data); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	ctx := r.Context()
	start := time.Now()
	if err := s.finance.AnalyzeFinance(ctx, req.Code, req.Market, sink); err != nil {
		log.Printf("[finance-analyze] code=%s 失败: %v", req.Code, err)
		_ = writeSSE(w, "error", map[string]string{"message": err.Error()})
		flusher.Flush()
		return
	}
	log.Printf("[finance-analyze] code=%s 完成 耗时=%dms", req.Code, time.Since(start).Milliseconds())
	_ = writeSSE(w, "done", struct{}{})
	flusher.Flush()
}
