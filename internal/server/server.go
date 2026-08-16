// Package server HTTP 层：Router + Handler
// 依赖 service 接口（Analyzer/Resolver/KlineProvider/NewsProvider），不依赖具体实现，便于 httptest 测试与替换。
package server

import (
	"context"
	"io/fs"
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

// KlineProvider 行情服务接口（由 internal/kline.GetKline 实现）
type KlineProvider interface {
	GetKline(ctx context.Context, market, code string, days int) (*types.Kline, error)
}

// NewsProvider 资讯服务接口（由 internal/news.GetNews 实现）
type NewsProvider interface {
	GetNews(ctx context.Context, keyword string, count int) ([]types.NewsItem, error)
}

// KnowledgeProvider 知识库搜索接口（由 *zhihu.Client.KnowledgeSearch 实现）
type KnowledgeProvider interface {
	KnowledgeSearch(ctx context.Context, query string, kbIDs []string, limit int) ([]types.KnowledgeItem, error)
}

// HotProvider 热门榜服务接口（由 internal/hot 实现）
type HotProvider interface {
	GetHot(ctx context.Context, typ string, count int) ([]types.HotItem, error)
	GetSectorStocks(ctx context.Context, code string, count int) ([]types.HotItem, error)
}

// KeyService 密钥服务接口：下发 RSA 公钥 + 接收前端加密提交的用户密钥（由入口 keybox 实现）
type KeyService interface {
	// PublicKeyPEM 返回公钥 PEM，前端用它加密用户填写的密钥
	PublicKeyPEM() string
	// DecryptOAEPBase64 用私钥解密前端提交的 base64(RSA-OAEP 密文)；空串输入返回空
	DecryptOAEPBase64(b64 string) ([]byte, error)
	// UpdateKeys 接收解密后的用户密钥并热更新（deepseekKey/zhihuSecret 为空表示该项跳过）
	UpdateKeys(deepseekKey, zhihuSecret string) error
}

// Server HTTP 层（依赖注入：实现在 cmd/server.main 组装）
type Server struct {
	analyzer      Analyzer
	resolver      Resolver
	klineProvider KlineProvider
	newsProvider  NewsProvider
	hotProvider   HotProvider
	knowledge     KnowledgeProvider
	keyService    KeyService
	frontend      fs.FS // 前端资源（go:embed，由入口注入）
}

// New 创建 Server
func New(analyzer Analyzer, resolver Resolver, klineProvider KlineProvider, newsProvider NewsProvider, hotProvider HotProvider, knowledge KnowledgeProvider, keyService KeyService, frontend fs.FS) *Server {
	return &Server{
		analyzer:      analyzer,
		resolver:      resolver,
		klineProvider: klineProvider,
		newsProvider:  newsProvider,
		hotProvider:   hotProvider,
		knowledge:     knowledge,
		keyService:    keyService,
		frontend:      frontend,
	}
}

// Routes 注册路由（Router 层）
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/resolve", s.handleResolve)         // 探针：股票识别（无需密钥）
	mux.HandleFunc("GET /api/kline", s.handleKline)             // 行情：报价 + 日K线
	mux.HandleFunc("GET /api/news", s.handleNews)               // 资讯：相关新闻（辅助）
	mux.HandleFunc("GET /api/hot", s.handleHot)                 // 热门：股票/板块榜
	mux.HandleFunc("GET /api/knowledge", s.handleKnowledge)     // 知识库搜索：股票讨论
	mux.HandleFunc("GET /api/config/pubkey", s.handlePubKey)    // 密钥箱：下发 RSA 公钥
	mux.HandleFunc("POST /api/config/keys", s.handleUpdateKeys) // 密钥箱：接收加密提交的用户密钥
	mux.HandleFunc("POST /api/ask", s.handleAsk)                // SSE：完整分析
	mux.Handle("GET /", http.FileServer(http.FS(s.frontend)))   // 前端：index.html + css/js 静态资源
	return mux
}
