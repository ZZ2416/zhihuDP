package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"zhihudp/internal/types"
)

// handleAsk POST /api/ask：SSE 流式返回分析事件
func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Stock string `json:"stock"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体必须是 JSON"})
		return
	}
	if strings.TrimSpace(req.Stock) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "stock is required"})
		return
	}

	// SSE 响应头
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

	// 客户端断开（r.Context()）即取消 agent 运行，防 goroutine 泄漏/白烧 token
	ctx := r.Context()
	start := time.Now()
	if err := s.analyzer.RunAnalysis(ctx, req.Stock, sink); err != nil {
		log.Printf("[ask] stock=%q 失败: %v", req.Stock, err)
		_ = writeSSE(w, "error", map[string]string{"message": err.Error()})
		flusher.Flush()
		return
	}
	log.Printf("[ask] stock=%q 完成 耗时=%dms", req.Stock, time.Since(start).Milliseconds())
	_ = writeSSE(w, "done", struct{}{})
	flusher.Flush()
}
