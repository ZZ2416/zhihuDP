package server

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"zhihudp/internal/types"
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
	// 链接可用性校验：空链接 / 非法 URL 的条目不展示（见 filterInvalidLinks 说明）
	items = filterInvalidLinks(items)
	log.Printf("[knowledge] q=%s count=%d 耗时=%dms", q, len(items), elapsed)
	writeJSON(w, http.StatusOK, items)
}

// filterInvalidLinks 过滤不可用链接的条目：空链接 / 非法 URL 直接不展示。
// 说明：知乎 WAF 对非浏览器请求一律返回 403（有效链接亦然），服务端无法做真实 HTTP 状态探测；
// 浏览器端受 CORS 限制也无法读取跨域状态码。因此以「链接存在且格式合法」为可用性判定，
// 链接均指向知乎官方域名，内容失效由知乎删除页兜底（前端另有 no-cors 网络可达性探测）。
func filterInvalidLinks(items []types.KnowledgeItem) []types.KnowledgeItem {
	out := items[:0]
	for _, it := range items {
		if validLink(it.OriginUrl) {
			out = append(out, it)
		}
	}
	return out
}

// validLink 判定链接格式可用：http/https + 合法 host
func validLink(u string) bool {
	if u == "" {
		return false
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return parsed.Host != ""
}

// defaultKnowledgeBaseID 默认股票讨论知识库（可由 server 构造注入）
func (s *Server) defaultKnowledgeBaseID() string { return "7520243014858214186" }
