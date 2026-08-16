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
	// 会话配额：每次打开页面 20 次 API 调用机会（与 /api/ask 共享）
	if _, ok := s.consumeQuota(w, r); !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "本次访问的 API 调用次数已用完（20 次），请重新打开页面"})
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

	// 捕获一期分析快照（二期对话上下文用）：stock 事件拿 code、sentiment 事件拿情绪结果、delta 累积分析文本
	var (
		curStock     types.StockInfo
		curSentiment *types.SentimentResult
		analysisText strings.Builder
	)
	sink := func(ev types.Event) error {
		switch ev.Type {
		case "stock":
			if info, ok := ev.Data.(*types.StockInfo); ok {
				curStock = *info
			}
		case "sentiment":
			if s, ok := ev.Data.(*types.SentimentResult); ok {
				curSentiment = s
			}
		case "delta":
			if m, ok := ev.Data.(map[string]string); ok {
				analysisText.WriteString(m["text"])
			}
		}
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

	// 二期：保存分析快照（情绪 + 最终分析文本），供「与看山对话」使用
	if s.chatProvider != nil && curStock.Code != "" {
		s.chatProvider.SetSnapshot(curStock.Code, curStock, curSentiment, analysisText.String())
	}
}
