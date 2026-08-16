package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"
)

// cfg 包级只读配置：main() 启动时加载，之后不再修改（并发安全）
var cfg *Config

func main() {
	configPath := flag.String("config", "config.yaml", "配置文件路径（默认 config.yaml）")
	flag.Parse()

	c, err := LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	cfg = c
	log.Printf("配置加载完成: %s", cfg.String())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/resolve", handleResolve) // M0 探针
	mux.HandleFunc("GET /", handleIndex)              // 前端入口（M2 托管 static/index.html）

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("zhihuDP 启动完成，访问 http://localhost%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// handleResolve M0 探针：脱离 LLM 直接验证第三方股票识别接口连通性
func handleResolve(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "q is required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	start := time.Now()
	info, err := ResolveStock(ctx, q)
	elapsed := time.Since(start).Milliseconds()

	switch {
	case errors.Is(err, ErrStockNotFound):
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

// handleIndex M0 占位页面；M2 改为托管 static/index.html
func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, "<h1>zhihuDP — 知乎股票情绪分析</h1><p>M0 骨架已就绪</p><p>探针测试：<code>GET /api/resolve?q=茅台</code></p>")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
