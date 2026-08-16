// Package server HTTP 层：Router + Handler
// 依赖 service 接口（Analyzer/Resolver），不依赖具体实现，便于 httptest 测试与替换。
package server

import (
	"context"
	"net/http"

	"zhihudp/internal/types"
)

// Analyzer 分析服务接口（由 internal/agent.RunAnalysis 实现）
type Analyzer interface {
	RunAnalysis(ctx context.Context, stock string, sink func(types.Event) error) error
}

// Resolver 股票识别服务接口（由 internal/stock.Resolve 实现）
type Resolver interface {
	Resolve(ctx context.Context, q string) (*types.StockInfo, error)
}

// Server HTTP 层（依赖注入：实现在 cmd/server.main 组装）
type Server struct {
	analyzer  Analyzer
	resolver  Resolver
	indexHTML []byte // 前端（go:embed，由入口注入）
}

// New 创建 Server
func New(analyzer Analyzer, resolver Resolver, indexHTML []byte) *Server {
	return &Server{analyzer: analyzer, resolver: resolver, indexHTML: indexHTML}
}

// Routes 注册路由（Router 层）
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/resolve", s.handleResolve) // 探针：股票识别（无需密钥）
	mux.HandleFunc("POST /api/ask", s.handleAsk)        // SSE：完整分析
	mux.HandleFunc("GET /", s.handleIndex)              // 前端入口
	return mux
}
