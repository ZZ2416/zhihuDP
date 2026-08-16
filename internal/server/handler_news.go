package server

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"
)

// handleNews GET /api/news?keyword=&count=：相关资讯（辅助数据，展示不进 LLM）
func (s *Server) handleNews(w http.ResponseWriter, r *http.Request) {
	keyword := r.URL.Query().Get("keyword")
	if keyword == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "keyword is required"})
		return
	}
	count := 5
	if c := r.URL.Query().Get("count"); c != "" {
		n, err := strconv.Atoi(c)
		if err != nil || n < 1 || n > 10 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "count 需为 1-10 的整数"})
			return
		}
		count = n
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	start := time.Now()
	items, err := s.newsProvider.GetNews(ctx, keyword, count)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		log.Printf("[news] keyword=%s 失败: %v 耗时=%dms", keyword, err, elapsed)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	log.Printf("[news] keyword=%s count=%d 耗时=%dms", keyword, len(items), elapsed)
	writeJSON(w, http.StatusOK, items)
}
