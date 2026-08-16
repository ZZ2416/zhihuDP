package server

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"
)

// handleKnowledge GET /api/knowledge?q=股票&limit=10：知识库搜索（股票讨论方形卡片数据）
func (s *Server) handleKnowledge(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "q is required"})
		return
	}
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 || n > 10 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit 需为 1-10 的整数"})
			return
		}
		limit = n
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	kbIDs := []string{s.defaultKnowledgeBaseID()}
	start := time.Now()
	items, err := s.knowledge.KnowledgeSearch(ctx, q, kbIDs, limit)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		log.Printf("[knowledge] q=%s 失败: %v 耗时=%dms", q, err, elapsed)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	log.Printf("[knowledge] q=%s count=%d 耗时=%dms", q, len(items), elapsed)
	writeJSON(w, http.StatusOK, items)
}

// defaultKnowledgeBaseID 默认股票讨论知识库（可由 server 构造注入）
func (s *Server) defaultKnowledgeBaseID() string { return "7520243014858214186" }
