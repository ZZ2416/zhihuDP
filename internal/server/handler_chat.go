// 二期追问对话 HTTP 处理器：POST /api/chat（SSE 流式，AI 看山）
package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"zhihudp/internal/types"
)

// handleChatReset POST /api/chat/reset：清空某股票的对话会话
func (s *Server) handleChatReset(w http.ResponseWriter, r *http.Request) {
	if s.chatProvider == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "对话服务未启用"})
		return
	}
	var req struct {
		Stock string `json:"stock"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体必须是 JSON"})
		return
	}
	if strings.TrimSpace(req.Stock) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "stock 不能为空"})
		return
	}
	s.chatProvider.Reset(strings.TrimSpace(req.Stock))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleChat POST /api/chat：AI 看山追问对话（SSE 事件：delta → done / error）
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if s.chatProvider == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "对话服务未启用"})
		return
	}
	var req struct {
		Stock   string `json:"stock"`   // 股票代码（必填）
		Market  string `json:"market"`  // 市场（可选，用于取行情）
		Message string `json:"message"` // 追问内容（必填，≤500 字）
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体必须是 JSON"})
		return
	}
	req.Stock = strings.TrimSpace(req.Stock)
	req.Message = strings.TrimSpace(req.Message)
	if req.Stock == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "stock 不能为空"})
		return
	}
	if req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message 不能为空"})
		return
	}
	if len([]rune(req.Message)) > 500 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message 过长（≤500 字）"})
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

	ctx := r.Context()
	start := time.Now()
	if err := s.chatProvider.Chat(ctx, req.Stock, req.Market, req.Message, sink); err != nil {
		log.Printf("[chat] stock=%q 失败: %v", req.Stock, err)
		_ = writeSSE(w, "error", map[string]string{"message": err.Error()})
		flusher.Flush()
		return
	}
	log.Printf("[chat] stock=%q 完成 耗时=%dms", req.Stock, time.Since(start).Milliseconds())
	_ = writeSSE(w, "done", struct{}{})
	flusher.Flush()
}
