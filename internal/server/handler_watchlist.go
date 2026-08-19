// 自选池 HTTP 处理器：GET /api/watchlist + POST /api/watchlist/add|remove
package server

import (
	"encoding/json"
	"net/http"
	"strings"
)

// handleWatchlist GET /api/watchlist：返回自选池列表
func (s *Server) handleWatchlist(w http.ResponseWriter, r *http.Request) {
	if s.watchlist == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "自选池服务未启用"})
		return
	}
	writeJSON(w, http.StatusOK, s.watchlist.List())
}

// handleWatchlistAdd POST /api/watchlist/add {"code":"600519","market":"沪A"}
func (s *Server) handleWatchlistAdd(w http.ResponseWriter, r *http.Request) {
	if s.watchlist == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "自选池服务未启用"})
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
	if req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code 不能为空"})
		return
	}
	remain, err := s.watchlist.Add(req.Code, req.Market)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "remain": remain})
}

// handleWatchlistRemove POST /api/watchlist/remove {"code":"600519"}
func (s *Server) handleWatchlistRemove(w http.ResponseWriter, r *http.Request) {
	if s.watchlist == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "自选池服务未启用"})
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体必须是 JSON"})
		return
	}
	if err := s.watchlist.Remove(strings.TrimSpace(req.Code)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
