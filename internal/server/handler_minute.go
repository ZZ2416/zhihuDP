// 分时 HTTP 处理器：GET /api/minute?code=&market=（当日分时，展示数据）
package server

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"
)

// handleMinute GET /api/minute?code=600519&market=沪A
func (s *Server) handleMinute(w http.ResponseWriter, r *http.Request) {
	if s.minute == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "分时服务未启用"})
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code 不能为空"})
		return
	}
	market := strings.TrimSpace(r.URL.Query().Get("market"))

	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()

	start := time.Now()
	res, err := s.minute.GetMinute(ctx, market, code)
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		log.Printf("[minute] code=%s 失败: %v 耗时=%dms", code, err, elapsed)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	log.Printf("[minute] code=%s 点数=%d 耗时=%dms", code, len(res.Points), elapsed)
	writeJSON(w, http.StatusOK, res)
}
