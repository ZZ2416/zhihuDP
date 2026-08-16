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
	"zhihudp/internal/config"
	"zhihudp/internal/kline"
	"zhihudp/internal/sentiment"
	"zhihudp/internal/server"
	"zhihudp/internal/stock"
	"zhihudp/internal/types"
	"zhihudp/web"
	"zhihudp/internal/zhihu"
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
	deps := buildDeps(cfg)
	srv := server.New(
		analyzerFunc(func(ctx context.Context, stock string, sink func(types.Event) error) error {
			return agent.RunAnalysis(ctx, stock, deps, sink)
		}),
		resolverFunc(stock.Resolve),
		klineProviderFunc(kline.GetKline),
		web.FS, // 前端资源（go:embed 内嵌）
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
	_ server.Analyzer      = (analyzerFunc)(nil)
	_ server.Resolver      = (resolverFunc)(nil)
	_ server.KlineProvider = (klineProviderFunc)(nil)
)

// klineProviderFunc 适配器：函数实现 → server.KlineProvider 接口
type klineProviderFunc func(ctx context.Context, market, code string, days int) (*types.Kline, error)

func (f klineProviderFunc) GetKline(ctx context.Context, market, code string, days int) (*types.Kline, error) {
	return f(ctx, market, code, days)
}

// buildDeps 组装 agent 依赖（业务层接线点）
func buildDeps(cfg *config.Config) agent.Deps {
	zhClient := zhihu.New(cfg.Zhihu)
	return agent.Deps{
		ResolveStock: stock.Resolve,
		AnalyzeSentiment: func(ctx context.Context, code, name string) (*types.SentimentResult, error) {
			return sentiment.Analyze(ctx, code, name, zhClient, cfg.DeepSeek)
		},
		DeepSeek: cfg.DeepSeek,
	}
}

// runCLI 命令行模式：跑一次分析，打印事件
func runCLI(query string, cfg *config.Config) {
	deps := buildDeps(cfg)
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
