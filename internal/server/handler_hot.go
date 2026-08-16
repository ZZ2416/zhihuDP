package server

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"zhihudp/internal/types"
)

// handleHot GET /api/hot?type=stock|sector|sector_stock&count=&code=
// 热门榜（展示数据，不进 LLM）
func (s *Server) handleHot(w http.ResponseWriter, r *http.Request) {
	typ := r.URL.Query().Get("type")
	if typ != "stock" && typ != "sector" && typ != "sector_stock" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "type 需为 stock / sector / sector_stock"})
		return
	}

	count := 8
	if typ == "sector" {
		count = 6
	}
	if c := r.URL.Query().Get("count"); c != "" {
		n, err := strconv.Atoi(c)
		if err != nil || n < 1 || n > 20 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "count 需为 1-20 的整数"})
			return
		}
		count = n
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	start := time.Now()
	var (
		items []types.HotItem
		err   error
	)
	if typ == "sector_stock" {
		code := r.URL.Query().Get("code")
		if code == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sector_stock 需要 code 参数（板块代码）"})
			return
		}
		items, err = s.hotProvider.GetSectorStocks(ctx, code, count)
	} else {
		items, err = s.hotProvider.GetHot(ctx, typ, count)
	}
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		log.Printf("[hot] type=%s 失败: %v 耗时=%dms", typ, err, elapsed)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	log.Printf("[hot] type=%s count=%d 耗时=%dms", typ, len(items), elapsed)
	writeJSON(w, http.StatusOK, items)
}
