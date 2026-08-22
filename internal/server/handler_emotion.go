// 情绪分析 HTTP 处理器：POST /api/emotion/analyze（情绪 AI 解读 SSE）
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

// EmotionProvider 情绪服务接口（由入口 emotionProvider 实现）
type EmotionProvider interface {
	// Analyze 拉取情绪数据并流式输出情绪解读（SSE 事件经 sink 转发）
	Analyze(ctx context.Context, code, market string, sink func(types.Event) error) error
}

// handleEmotionAnalyze POST /api/emotion/analyze {"code":"600519","market":"沪A"}
func (s *Server) handleEmotionAnalyze(w http.ResponseWriter, r *http.Request) {
	if s.emotion == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "情绪服务未启用"})
		return
	}
	var req struct {
		Code   string `json:"code"`
		Market string `json:"market"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体必须是 JSON"})
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	req.Market = strings.TrimSpace(req.Market)
	if req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code 不能为空"})
		return
	}

	// 会话配额（与 ask/chat/finance-analyze 共享）
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
	if err := s.emotion.Analyze(ctx, req.Code, req.Market, sink); err != nil {
		log.Printf("[emotion] code=%s 失败: %v", req.Code, err)
		_ = writeSSE(w, "error", map[string]string{"message": err.Error()})
		flusher.Flush()
		return
	}
	log.Printf("[emotion] code=%s 完成 耗时=%dms", req.Code, time.Since(start).Milliseconds())
	_ = writeSSE(w, "done", struct{}{})
	flusher.Flush()
}
