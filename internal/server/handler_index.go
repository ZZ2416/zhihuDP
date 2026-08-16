package server

import "net/http"

// handleIndex GET /：托管前端（go:embed 内嵌，由入口注入）
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(s.indexHTML)
}
