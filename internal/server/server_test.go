package server

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// fakeKeyService 密钥箱桩：返回固定公钥，记录解密结果
type fakeKeyService struct{}

func (fakeKeyService) PublicKeyPEM() string {
	return "-----BEGIN PUBLIC KEY-----\ntest\n-----END PUBLIC KEY-----"
}

func (fakeKeyService) DecryptOAEPBase64(b64 string) ([]byte, error) { return []byte(b64), nil }

func (fakeKeyService) UpdateKeys(deepseekKey, zhihuSecret string) error { return nil }

func (fakeKeyService) PersistKeys(deepseekKeyEnc, zhihuSecretEnc string) error { return nil }

// fakeChatProvider 二期对话桩：直接回一段 delta
type fakeChatProvider struct{}

func (fakeChatProvider) Chat(_ context.Context, _, _, _ string, sink func(types.Event) error) error {
	return sink(types.Event{Type: "delta", Data: map[string]string{"text": "看山觉得…"}})
}

func (fakeChatProvider) SetSnapshot(_ string, _ types.StockInfo, _ *types.SentimentResult, _ string) {
}

func (fakeChatProvider) Reset(_ string) {}

func newTestServer() *Server {
	frontend := fstest.MapFS{
		"index.html":    {Data: []byte("<html>test</html>")},
		"css/style.css": {Data: []byte("body{}")},
		"js/app.js":     {Data: []byte("// app")},
	}
	return New(fakeAnalyzer{}, fakeResolver{}, fakeKlineProvider{}, fakeNewsProvider{}, fakeHotProvider{}, fakeKnowledgeProvider{}, fakeKeyService{}, fakeChatProvider{}, "", "", frontend)
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

type fakeKnowledgeProvider struct{}

func (fakeKnowledgeProvider) KnowledgeSearch(_ context.Context, _ string, _ []string, _ int) ([]types.KnowledgeItem, error) {
	return []types.KnowledgeItem{{DocName: "测试讨论", OriginUrl: "https://www.zhihu.com/", Content: []string{"股票讨论内容"}}}, nil
}

func TestFilterInvalidLinks(t *testing.T) {
	items := []types.KnowledgeItem{
		{DocName: "有链接", OriginUrl: "https://zhuanlan.zhihu.com/p/1993980027537229643"},
		{DocName: "空链接", OriginUrl: ""},
		{DocName: "非http", OriginUrl: "javascript:alert(1)"},
		{DocName: "无host", OriginUrl: "https://"},
		{DocName: "非法url", OriginUrl: "ht tp://x"},
	}
	got := filterInvalidLinks(items)
	if len(got) != 1 || got[0].DocName != "有链接" {
		t.Fatalf("期望仅保留合法链接条目，实际 %d 条: %+v", len(got), got)
	}
}

func TestQuotaStore(t *testing.T) {
	q := NewQuota(3)
	// 3 次内放行
	for i := 1; i <= 3; i++ {
		remain, ok := q.Consume("tok-a")
		if !ok || remain != 3-i {
			t.Fatalf("第 %d 次应放行，剩余 %d", i, remain)
		}
	}
	// 第 4 次超限
	if _, ok := q.Consume("tok-a"); ok {
		t.Fatal("第 4 次应超限")
	}
	// 新 token 重新配额
	if remain, ok := q.Consume("tok-b"); !ok || remain != 2 {
		t.Fatalf("新 token 应重新配额，剩余 %d", remain)
	}
	// 空 token 拒绝
	if _, ok := q.Consume(""); ok {
		t.Fatal("空 token 应拒绝")
	}
}

func TestAskQuotaExceeded(t *testing.T) {
	s := newTestServer()
	// 无 cookie → 每次都是新会话首次分配（测试直接构造小配额便于验证）
	s.quota = NewQuota(1)
	req := httptest.NewRequest(http.MethodPost, "/api/ask", strings.NewReader(`{"stock":"茅台"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("首次应 200，实际 %d", rec.Code)
	}
	// 同一 token 第二次 → 403
	req2 := httptest.NewRequest(http.MethodPost, "/api/ask", strings.NewReader(`{"stock":"茅台"}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.AddCookie(&http.Cookie{Name: quotaTokenCookie, Value: "same-token"})
	rec2 := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("首次带 token 应 200，实际 %d", rec2.Code)
	}
	req3 := httptest.NewRequest(http.MethodPost, "/api/ask", strings.NewReader(`{"stock":"茅台"}`))
	req3.Header.Set("Content-Type", "application/json")
	req3.AddCookie(&http.Cookie{Name: quotaTokenCookie, Value: "same-token"})
	rec3 := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusForbidden {
		t.Fatalf("超限应 403，实际 %d", rec3.Code)
	}
}

func TestMediaTokenProtection(t *testing.T) {
	// 构造带媒体目录/令牌的 server（临时目录放一个测试视频）
	dir := t.TempDir()
	video := filepath.Join(dir, "demo.mp4")
	if err := os.WriteFile(video, []byte("fake-video-data"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newTestServer()
	s.mediaDir = dir
	s.mediaToken = "secret-token"

	// 无 token → 403
	req := httptest.NewRequest(http.MethodGet, "/media/player?f=demo.mp4", nil)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("无 token 应 403，实际 %d", rec.Code)
	}
	// 错 token → 403
	req = httptest.NewRequest(http.MethodGet, "/media/player?t=wrong&f=demo.mp4", nil)
	rec = httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("错 token 应 403，实际 %d", rec.Code)
	}
	// 正确 token → 播放页 200 且含禁下载属性
	req = httptest.NewRequest(http.MethodGet, "/media/player?t=secret-token&f=demo.mp4", nil)
	rec = httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("正确 token 应 200，实际 %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "nodownload") || !strings.Contains(body, "oncontextmenu") {
		t.Errorf("播放页应含禁下载/禁右键属性")
	}
	// 视频流正确 token → 200 内容一致
	req = httptest.NewRequest(http.MethodGet, "/media/file?t=secret-token&f=demo.mp4", nil)
	rec = httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "fake-video-data" {
		t.Fatalf("视频流异常: %d %q", rec.Code, rec.Body.String())
	}
	// 路径穿越防护：f=../config.yaml 应被拒绝（base 化后不存在）
	req = httptest.NewRequest(http.MethodGet, "/media/player?t=secret-token&f=..%2F..%2Fetc%2Fpasswd", nil)
	rec = httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("路径穿越应 404，实际 %d", rec.Code)
	}
}
