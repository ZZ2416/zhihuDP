// 入口：加载配置 → 组装依赖与 HTTP 层 → 启动服务 / CLI 模式
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"zhihudp/internal/agent"
	"zhihudp/internal/chat"
	"zhihudp/internal/config"
	"zhihudp/internal/hot"
	"zhihudp/internal/keybox"
	"zhihudp/internal/kline"
	"zhihudp/internal/news"
	"zhihudp/internal/sentiment"
	"zhihudp/internal/server"
	"zhihudp/internal/stock"
	"zhihudp/internal/types"
	"zhihudp/internal/zhihu"
	"zhihudp/web"
)

func main() {
	configPath := flag.String("config", "config.yaml", "配置文件路径（默认 config.yaml）")
	query := flag.String("q", "", "CLI 模式：运行一次分析并打印事件（如 -q 茅台）")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	log.Printf("配置加载完成: %s", cfg.String())

	// CLI 模式（快速验证用，不启动 HTTP）
	if *query != "" {
		runCLI(*query, cfg)
		return
	}

	// 组装依赖 + HTTP 层（依赖注入：各包无全局状态）
	zhClient := zhihu.New(cfg.Zhihu)
	kb, err := keybox.New()
	if err != nil {
		log.Fatalf("初始化密钥箱失败: %v", err)
	}
	ks := &keyService{KeyBox: kb, cfg: cfg, zhClient: zhClient}
	deps := buildDeps(cfg, zhClient)

	// 二期：看山追问对话服务（会话按股票隔离，快照由 /api/ask 捕获）
	chatSvc := chat.New(chat.NewStore(10), chat.Deps{
		Quote: func(ctx context.Context, market, code string) (*types.Quote, error) {
			k, err := kline.GetKline(ctx, market, code, 1) // 仅取报价段，不取 K 线序列
			if err != nil {
				return nil, err
			}
			return &k.Quote, nil
		},
		Knowledge: func(ctx context.Context, query string, limit int) ([]types.KnowledgeItem, error) {
			return zhClient.KnowledgeSearch(ctx, query, []string{"7520243014858214186"}, limit)
		},
		ChatAgent: func(ctx context.Context, facts types.ChatFacts, history []types.ChatMessage, message string, sink func(types.Event) error) error {
			return agent.Chat(ctx, facts, history, message, deps, sink)
		},
	})

	srv := server.New(
		analyzerFunc(func(ctx context.Context, stock string, sink func(types.Event) error) error {
			return agent.RunAnalysis(ctx, stock, deps, sink)
		}),
		resolverFunc(stock.Resolve),
		klineProviderFunc(kline.GetKline),
		newsProviderFunc(news.GetNews),
		hotProviderFunc{getHot: hot.GetHot, getSectorStocks: hot.GetSectorStocks},
		knowledgeProviderFunc(func(ctx context.Context, query string, kbIDs []string, limit int) ([]types.KnowledgeItem, error) {
			return zhClient.KnowledgeSearch(ctx, query, kbIDs, limit)
		}),
		ks,      // 密钥箱：公钥下发 + 加密密钥热更新
		chatSvc, // 二期：看山追问对话
		web.FS,  // 前端资源（go:embed 内嵌）
	)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("zhihuDP 启动完成，访问 http://localhost%s", addr)
	if err := http.ListenAndServe(addr, srv.Routes()); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// analyzerFunc / resolverFunc 适配器：函数实现 → server 方法型接口
type analyzerFunc func(ctx context.Context, stock string, sink func(types.Event) error) error

func (f analyzerFunc) RunAnalysis(ctx context.Context, stock string, sink func(types.Event) error) error {
	return f(ctx, stock, sink)
}

type resolverFunc func(ctx context.Context, q string) (*types.StockInfo, error)

func (f resolverFunc) Resolve(ctx context.Context, q string) (*types.StockInfo, error) {
	return f(ctx, q)
}

// 编译期断言：适配器满足 server 接口（house style）
var (
	_ server.Analyzer          = (analyzerFunc)(nil)
	_ server.Resolver          = (resolverFunc)(nil)
	_ server.KlineProvider     = (klineProviderFunc)(nil)
	_ server.NewsProvider      = (newsProviderFunc)(nil)
	_ server.HotProvider       = hotProviderFunc{}
	_ server.KnowledgeProvider = (knowledgeProviderFunc)(nil)
)

// klineProviderFunc 适配器：函数实现 → server.KlineProvider 接口
type klineProviderFunc func(ctx context.Context, market, code string, days int) (*types.Kline, error)

func (f klineProviderFunc) GetKline(ctx context.Context, market, code string, days int) (*types.Kline, error) {
	return f(ctx, market, code, days)
}

// newsProviderFunc 适配器：函数实现 → server.NewsProvider 接口
type newsProviderFunc func(ctx context.Context, keyword string, count int) ([]types.NewsItem, error)

func (f newsProviderFunc) GetNews(ctx context.Context, keyword string, count int) ([]types.NewsItem, error) {
	return f(ctx, keyword, count)
}

// knowledgeProviderFunc 适配器：函数实现 → server.KnowledgeProvider 接口
type knowledgeProviderFunc func(ctx context.Context, query string, kbIDs []string, limit int) ([]types.KnowledgeItem, error)

func (f knowledgeProviderFunc) KnowledgeSearch(ctx context.Context, query string, kbIDs []string, limit int) ([]types.KnowledgeItem, error) {
	return f(ctx, query, kbIDs, limit)
}

// hotProviderFunc 适配器：函数实现 → server.HotProvider 接口
type hotProviderFunc struct {
	getHot          func(ctx context.Context, typ string, count int) ([]types.HotItem, error)
	getSectorStocks func(ctx context.Context, code string, count int) ([]types.HotItem, error)
}

func (f hotProviderFunc) GetHot(ctx context.Context, typ string, count int) ([]types.HotItem, error) {
	return f.getHot(ctx, typ, count)
}

func (f hotProviderFunc) GetSectorStocks(ctx context.Context, code string, count int) ([]types.HotItem, error) {
	return f.getSectorStocks(ctx, code, count)
}

// keyService 密钥箱服务：RSA 加解密 + 热更新运行中的配置（实现 server.KeyService）
type keyService struct {
	*keybox.KeyBox
	cfg      *config.Config
	zhClient *zhihu.Client
}

// UpdateKeys 应用用户提交的密钥（空值表示该项未填写，保留原密钥）
func (k *keyService) UpdateKeys(deepseekKey, zhihuSecret string) error {
	if deepseekKey != "" {
		k.cfg.DeepSeek.APIKey = deepseekKey
	}
	if zhihuSecret != "" {
		k.cfg.Zhihu.AccessSecret = zhihuSecret
		k.zhClient.UpdateKeys(zhihuSecret)
	}
	return nil
}

// 编译期断言：keyService 满足 server.KeyService
var _ server.KeyService = (*keyService)(nil)

// buildDeps 组装 agent 依赖（业务层接线点）
func buildDeps(cfg *config.Config, zhClient *zhihu.Client) agent.Deps {
	return agent.Deps{
		ResolveStock: stock.Resolve,
		AnalyzeSentiment: func(ctx context.Context, code, name string) (*types.SentimentResult, error) {
			return sentiment.Analyze(ctx, code, name, zhClient, cfg.DeepSeek)
		},
		// getter：每次调用读取最新配置（支持弹窗热更新密钥后立即生效）
		DeepSeek: func() config.DeepSeekConfig { return cfg.DeepSeek },
	}
}

// runCLI 命令行模式：跑一次分析，打印事件
func runCLI(query string, cfg *config.Config) {
	deps := buildDeps(cfg, zhihu.New(cfg.Zhihu))
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	err := agent.RunAnalysis(ctx, query, deps, func(ev types.Event) error {
		b, _ := json.Marshal(ev.Data)
		fmt.Printf("[%s] %s\n", ev.Type, b)
		return nil
	})
	if err != nil {
		fmt.Printf("[error] %v\n", err)
	}
}
