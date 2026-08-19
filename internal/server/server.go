// Package server HTTP 层：Router + Handler
// 依赖 service 接口（Analyzer/Resolver/KlineProvider/NewsProvider），不依赖具体实现，便于 httptest 测试与替换。
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io/fs"
	"net/http"
	"sync"

	"zhihudp/internal/types"
)

// quotaTokenCookie 会话配额令牌 Cookie 名：每次「打开页面」（新会话）分配 20 次 API 调用机会
const quotaTokenCookie = "zhihudp_token"

// QuotaStore 会话配额：token → 剩余调用次数（内存；重启重置，demo 可接受）
type QuotaStore struct {
	mu     sync.Mutex
	tokens map[string]int
	limit  int
}

// NewQuota 创建配额存储
func NewQuota(limit int) *QuotaStore {
	if limit < 1 {
		limit = 20
	}
	return &QuotaStore{tokens: map[string]int{}, limit: limit}
}

// Consume 扣减一次调用；返回 (剩余次数, 是否允许)。false = 超限。
func (q *QuotaStore) Consume(token string) (int, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if token == "" {
		return 0, false
	}
	remain, ok := q.tokens[token]
	if !ok {
		remain = q.limit // 未知 token：视为新会话，首次分配（兜底）
	}
	if remain <= 0 {
		return 0, false
	}
	remain--
	q.tokens[token] = remain
	return remain, true
}

// newQuotaToken 生成随机会话令牌
func newQuotaToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

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
	// UpdateKeys 接收解密后的 DeepSeek 密钥并热更新
	UpdateKeys(deepseekKey string) error
	// PersistKeys 把加密后的密钥密文持久化到 config.yaml（只写 *_enc 字段，不落明文）
	PersistKeys(deepseekKeyEnc string) error
}

// VideoProvider 视频资讯服务接口（由入口 videoProvider 实现）
type VideoProvider interface {
	GetVideos(ctx context.Context, keyword string, count int) ([]types.VideoItem, error)
}

// MinuteProvider 分时数据服务接口（由入口 minuteProvider 实现）
type MinuteProvider interface {
	GetMinute(ctx context.Context, market, code string) (*types.MinuteResult, error)
}

// FinanceProvider 财务解析服务接口（由入口 financeProvider 实现）
type FinanceProvider interface {
	// GetFinance 获取股票财务指标（5 年年报 + 最新报告期）
	GetFinance(ctx context.Context, code, market string) (*types.FinanceResult, error)
	// AnalyzeFinance 财报 AI 解析（SSE 事件经 sink 转发）
	AnalyzeFinance(ctx context.Context, code, market string, sink func(types.Event) error) error
}

// ChatProvider 二期追问对话服务接口（由 internal/chat.Service 实现）
type ChatProvider interface {
	// Chat 处理一次追问：SSE 事件经 sink 转发（delta → done）
	Chat(ctx context.Context, code, market, message string, sink func(types.Event) error) error
	// SetSnapshot 一期 /api/ask 结束后写入结果快照（分析文本），供对话上下文使用
	SetSnapshot(code string, stock types.StockInfo, analysis string)
	// Reset 清空某股票的对话会话（前端「清空」按钮）
	Reset(code string)
}

// Server HTTP 层（依赖注入：实现在 cmd/server.main 组装）
type Server struct {
	analyzer      Analyzer
	resolver      Resolver
	klineProvider KlineProvider
	newsProvider  NewsProvider
	hotProvider   HotProvider
	keyService    KeyService
	chatProvider  ChatProvider
	finance       FinanceProvider
	minute        MinuteProvider
	video         VideoProvider
	fundamental   FundamentalProvider
	quota         *QuotaStore // 会话配额：每次打开页面 20 次 API 调用
	mediaDir      string      // 媒体目录（/media/ 播放）；空 = 禁用
	mediaToken    string      // 媒体访问令牌；空/不匹配 → 403（防未授权访问与转发）
	frontend      fs.FS       // 前端资源（go:embed，由入口注入）
}

// New 创建 Server
func New(analyzer Analyzer, resolver Resolver, klineProvider KlineProvider, newsProvider NewsProvider, hotProvider HotProvider, keyService KeyService, chatProvider ChatProvider, finance FinanceProvider, minute MinuteProvider, video VideoProvider, fundamental FundamentalProvider, mediaDir, mediaToken string, frontend fs.FS) *Server {
	return &Server{
		analyzer:      analyzer,
		resolver:      resolver,
		klineProvider: klineProvider,
		newsProvider:  newsProvider,
		hotProvider:   hotProvider,
		keyService:    keyService,
		chatProvider:  chatProvider,
		finance:       finance,
		minute:        minute,
		video:         video,
		fundamental:   fundamental,
		quota:         NewQuota(20), // 每次打开页面 20 次 API 调用机会
		mediaDir:      mediaDir,
		mediaToken:    mediaToken,
		frontend:      frontend,
	}
}

// Routes 注册路由（Router 层）
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/resolve", s.handleResolve)                 // 探针：股票识别（无需密钥）
	mux.HandleFunc("GET /api/kline", s.handleKline)                     // 行情：报价 + 日K线
	mux.HandleFunc("GET /api/news", s.handleNews)                       // 资讯：相关新闻（辅助）
	mux.HandleFunc("GET /api/hot", s.handleHot)                         // 热门：股票/板块榜
	mux.HandleFunc("GET /api/config/pubkey", s.handlePubKey)            // 密钥箱：下发 RSA 公钥
	mux.HandleFunc("POST /api/config/keys", s.handleUpdateKeys)         // 密钥箱：接收加密提交的用户密钥
	mux.HandleFunc("POST /api/ask", s.handleAsk)                        // SSE：完整分析
	mux.HandleFunc("POST /api/chat", s.handleChat)                      // SSE：二期看山追问对话
	mux.HandleFunc("POST /api/chat/reset", s.handleChatReset)           // 二期：清空某股票会话
	mux.HandleFunc("GET /api/finance", s.handleFinance)                 // 财务指标（展示数据）
	mux.HandleFunc("POST /api/finance/analyze", s.handleFinanceAnalyze) // 财报 AI 解析（SSE，计配额）
	mux.HandleFunc("GET /api/minute", s.handleMinute)                   // 分时数据（当日）
	mux.HandleFunc("GET /api/fundamental", s.handleFundamental)         // 基本面评分数据
	mux.HandleFunc("GET /api/video", s.handleVideo)                     // 视频资讯（B站，按时间/播放量）
	// 媒体播放（抖音式禁止转载）：token 校验 + 受保护播放页 + 视频流（Range 支持）
	if s.mediaDir != "" && s.mediaToken != "" {
		mux.HandleFunc("GET /media/player", s.handleMediaPlayer) // 播放页（禁下载/右键）
		mux.HandleFunc("GET /media/file", s.handleMediaFile)     // 视频流（token 校验）
	}
	// 首页：下发会话配额令牌 Cookie（每次打开页面 = 新会话 = 20 次 API 调用机会）
	mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie(quotaTokenCookie); err != nil {
			http.SetCookie(w, &http.Cookie{
				Name: quotaTokenCookie, Value: newQuotaToken(),
				Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
			})
		}
		http.FileServer(http.FS(s.frontend)).ServeHTTP(w, r)
	}))
	return mux
}

// consumeQuota 扣减会话配额；无 cookie 时自动分配新令牌（Set-Cookie）并计数。
// 超限返回 false（调用方返回 403）。
func (s *Server) consumeQuota(w http.ResponseWriter, r *http.Request) (int, bool) {
	token := ""
	if c, err := r.Cookie(quotaTokenCookie); err == nil {
		token = c.Value
	}
	if token == "" {
		token = newQuotaToken()
		http.SetCookie(w, &http.Cookie{
			Name: quotaTokenCookie, Value: token,
			Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		})
	}
	return s.quota.Consume(token)
}
