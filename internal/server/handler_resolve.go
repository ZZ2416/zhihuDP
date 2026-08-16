package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"zhihudp/internal/types"
)

// handleResolve GET /api/resolve?q=：探针，脱离 LLM 验证股票识别接口
func (s *Server) handleResolve(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "q is required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	start := time.Now()
	info, err := s.resolver.Resolve(ctx, q)
	elapsed := time.Since(start).Milliseconds()

	switch {
	case errors.Is(err, types.ErrStockNotFound):
		log.Printf("[resolve] q=%q 未找到 耗时=%dms", q, elapsed)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "未找到该股票，请检查名称/代码"})
	case err != nil:
		log.Printf("[resolve] q=%q 失败: %v 耗时=%dms", q, err, elapsed)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
	default:
		log.Printf("[resolve] q=%q -> %s %s(%s) 耗时=%dms", q, info.Name, info.Code, info.Market, elapsed)
		writeJSON(w, http.StatusOK, info)
	}
}
