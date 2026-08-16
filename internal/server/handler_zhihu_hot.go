package server

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"
)

// handleZhihuHot GET /api/zhihu-hot?count=：知乎热榜（主数据源，每 3h 更新）
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
	items, err := s.zhihuHot.HotList(ctx, count)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		log.Printf("[zhihu-hot] 失败: %v 耗时=%dms", err, elapsed)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	log.Printf("[zhihu-hot] count=%d 耗时=%dms", len(items), elapsed)
	writeJSON(w, http.StatusOK, items)
}
