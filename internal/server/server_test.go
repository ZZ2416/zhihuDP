package server

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"zhihudp/internal/types"
)

// fake 实现：验证 Handler 依赖接口而非具体实现
type fakeResolver struct{}

func (fakeResolver) Resolve(_ context.Context, q string) (*types.StockInfo, error) {
	if q == "茅台" {
		return &types.StockInfo{Code: "600519", Name: "贵州茅台", Market: "沪A"}, nil
	}
	return nil, types.ErrStockNotFound
}

type fakeAnalyzer struct{}

func (fakeAnalyzer) RunAnalysis(_ context.Context, _ string, sink func(types.Event) error) error {
	return sink(types.Event{Type: "delta", Data: map[string]string{"text": "ok"}})
}

type fakeKlineProvider struct{}

func (fakeKlineProvider) GetKline(_ context.Context, market, code string, days int) (*types.Kline, error) {
	return &types.Kline{
		Quote: types.Quote{Code: code, Name: "测试股", Price: 10.5, ChangePct: 1.2},
		Candles: []types.Candle{
			{Date: "2026-08-12", Open: 10, Close: 10.5, High: 10.8, Low: 9.9, Volume: 100},
		},
	}, nil
}

type fakeNewsProvider struct{}

func (fakeNewsProvider) GetNews(_ context.Context, _ string, _ int) ([]types.NewsItem, error) {
	return []types.NewsItem{{Title: "测试资讯", Url: "https://example.com", Date: "2026-08-14", Source: "东方财富"}}, nil
}

func newTestServer() *Server {
	frontend := fstest.MapFS{
		"index.html":      {Data: []byte("<html>test</html>")},
		"css/style.css":   {Data: []byte("body{}")},
		"js/app.js":       {Data: []byte("// app")},
	}
	return New(fakeAnalyzer{}, fakeResolver{}, fakeKlineProvider{}, fakeNewsProvider{}, fakeHotProvider{}, frontend)
}

var _ fs.FS = (fstest.MapFS)(nil)

func TestResolveHandler(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/resolve?q=%E8%8C%85%E5%8F%B0", nil)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "600519") {
		t.Errorf("响应应包含股票代码: %s", rec.Body.String())
	}
}

func TestResolveHandler_NotFound(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/resolve?q=zzz", nil)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("期望 404，实际 %d", rec.Code)
	}
}

func TestResolveHandler_EmptyQuery(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/resolve", nil)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望 400，实际 %d", rec.Code)
	}
}

func TestAskHandler_SSE(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/ask",
		strings.NewReader(`{"stock":"茅台"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("SSE 响应头错误: %s", ct)
	}
	if !strings.Contains(rec.Body.String(), "event: delta") {
		t.Errorf("SSE 应包含 delta 事件: %s", rec.Body.String())
	}
}

func TestAskHandler_EmptyStock(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/ask",
		strings.NewReader(`{"stock":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("空 stock 期望 400，实际 %d", rec.Code)
	}
}

func TestIndexHandler(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d", rec.Code)
	}
	if rec.Body.String() != "<html>test</html>" {
		t.Errorf("首页内容错误: %s", rec.Body.String())
	}
}

type fakeHotProvider struct{}

func (fakeHotProvider) GetHot(_ context.Context, typ string, _ int) ([]types.HotItem, error) {
	return []types.HotItem{{Code: "600519", Name: "贵州茅台", Price: 1341.99, ChangePct: -0.98, Type: typ}}, nil
}

func (fakeHotProvider) GetSectorStocks(_ context.Context, _ string, _ int) ([]types.HotItem, error) {
	return []types.HotItem{{Code: "002594", Name: "比亚迪", Price: 88.9, ChangePct: 1.2, Type: "stock"}}, nil
}


