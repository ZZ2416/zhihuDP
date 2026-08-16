// 入口：加载配置 → 组装依赖（zhihu.Client → sentiment → agent.Deps）→ HTTP 服务 / CLI 模式
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"zhihudp/internal/agent"
	"zhihudp/internal/config"
	"zhihudp/internal/sentiment"
	"zhihudp/internal/stock"
	"zhihudp/internal/types"
	"zhihudp/internal/web"
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

	// CLI 模式（快速验证用）
	if *query != "" {
		runCLI(*query, cfg)
		return
	}

	// 组装依赖（依赖注入：各包无全局状态）
	deps := buildDeps(cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/resolve", handleResolve) // 探针：股票识别（无需密钥）
	mux.HandleFunc("POST /api/ask", func(w http.ResponseWriter, r *http.Request) {
		handleAsk(w, r, deps)
	})
	mux.HandleFunc("GET /", handleIndex) // 前端入口（go:embed）

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("zhihuDP 启动完成，访问 http://localhost%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// buildDeps 组装 agent 依赖
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

// handleAsk POST /api/ask：SSE 流式返回分析事件
func handleAsk(w http.ResponseWriter, r *http.Request, deps agent.Deps) {
	var req struct {
		Stock string `json:"stock"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体必须是 JSON"})
		return
	}
	if strings.TrimSpace(req.Stock) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "stock is required"})
		return
	}

	// SSE 响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "SSE 不受支持"})
		return
	}

	sink := func(ev types.Event) error {
		if err := writeSSE(w, ev.Type, ev.Data); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	// 客户端断开（r.Context()）即取消 agent 运行，防 goroutine 泄漏/白烧 token
	ctx := r.Context()
	start := time.Now()
	if err := agent.RunAnalysis(ctx, req.Stock, deps, sink); err != nil {
		log.Printf("[ask] stock=%q 失败: %v", req.Stock, err)
		_ = writeSSE(w, "error", map[string]string{"message": err.Error()})
		flusher.Flush()
		return
	}
	log.Printf("[ask] stock=%q 完成 耗时=%dms", req.Stock, time.Since(start).Milliseconds())
	_ = writeSSE(w, "done", struct{}{})
	flusher.Flush()
}

// handleResolve GET /api/resolve?q=：探针，脱离 LLM 验证股票识别接口
func handleResolve(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "q is required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	start := time.Now()
	info, err := stock.Resolve(ctx, q)
	elapsed := time.Since(start).Milliseconds()

	switch {
	case errors.Is(err, stock.ErrNotFound):
		log.Printf("[resolve] q=%q 未找到 耗时=%dms", q, elapsed)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "未找到该股票，请检查名称/代码"})
	case err != nil:
		log.Printf("[resolve] q=%q 失败: %v 耗时=%dms", q, err, elapsed)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
	default:
		log.Printf("[resolve] q=%q -> %s %s(%s) 耗时=%dms", q, info.Name, info.Code, info.Market, elapsed)
		writeJSON(w, http.StatusOK, info)
	}
}

// handleIndex GET /：托管前端（go:embed 内嵌）
func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := web.FS.ReadFile("index.html")
	if err != nil {
		http.Error(w, "前端资源缺失", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func writeSSE(w http.ResponseWriter, event string, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
	return err
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
