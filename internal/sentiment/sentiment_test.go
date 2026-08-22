package sentiment

import (
	"context"
	"errors"
	"testing"

	"zhihudp/internal/config"
	"zhihudp/internal/types"
	"zhihudp/internal/zhihu"
)

// 编译期断言：*zhihu.Client 满足 Searcher 接口（house style）
var _ Searcher = (*zhihu.Client)(nil)

// fakeSearcher mock 知乎搜索（验证 Analyze 依赖接口而非具体实现）
type fakeSearcher struct {
	resp *zhihu.SearchResponse
	err  error
}

func (f fakeSearcher) Search(_ context.Context, _ string, _ int) (*zhihu.SearchResponse, error) {
	return f.resp, f.err
}

func TestComputeStrength(t *testing.T) {
	cases := []struct {
		name             string
		r                types.Ratio
		sample, heat     int
		wantMin, wantMax int
	}{
		{"一致性强+样本足", types.Ratio{Bull: 0.9, Bear: 0.05}, 50, 40, 7, 10},
		{"分歧大+样本足", types.Ratio{Bull: 0.5, Bear: 0.45}, 50, 40, 4, 7},
		{"一致性强+样本少", types.Ratio{Bull: 0.9, Bear: 0.05}, 3, 5, 4, 8},
	}
	for _, c := range cases {
		got := ComputeStrength(c.r, c.sample, c.heat)
		if got < c.wantMin || got > c.wantMax {
			t.Errorf("%s: ComputeStrength=%d 期望 [%d,%d]", c.name, got, c.wantMin, c.wantMax)
		}
	}
	// 边界钳制
	if s := ComputeStrength(types.Ratio{}, 0, 0); s < 1 || s > 10 {
		t.Errorf("边界钳制失败: %d", s)
	}
	if s := ComputeStrength(types.Ratio{Bull: 1, Bear: 0}, 1000, 1000); s < 1 || s > 10 {
		t.Errorf("上限钳制失败: %d", s)
	}
}

func TestAnalyze_NoKey_Degraded(t *testing.T) {
	// 无 deepseek key：降级而非报错（业务规则 §4.5）
	result, err := Analyze(context.Background(), "600519", "贵州茅台", fakeSearcher{}, config.DeepSeekConfig{})
	if err != nil {
		t.Fatalf("无密钥时应降级而非返回 error: %v", err)
	}
	if !result.Degraded {
		t.Errorf("无密钥时应 Degraded=true")
	}
	if result.Score != nil {
		t.Errorf("降级时 Score 应为 nil")
	}
}

func TestAnalyze_SearchError_Degraded(t *testing.T) {
	// 搜索失败：降级而非报错
	ds := config.DeepSeekConfig{APIKey: "fake-key", BaseURL: "https://api.deepseek.com"}
	result, err := Analyze(context.Background(), "600519", "贵州茅台",
		fakeSearcher{err: errors.New("网络超时")}, ds)
	if err != nil {
		t.Fatalf("搜索失败时应降级而非返回 error: %v", err)
	}
	if !result.Degraded {
		t.Errorf("搜索失败时应 Degraded=true")
	}
	if result.Score != nil {
		t.Errorf("降级时 Score 应为 nil")
	}
}

func TestAnalyze_EmptyResults_Degraded(t *testing.T) {
	// 无讨论结果：降级（样本不足）
	ds := config.DeepSeekConfig{APIKey: "fake-key", BaseURL: "https://api.deepseek.com"}
	resp := &zhihu.SearchResponse{Code: 0, Data: struct {
		HasMore bool         `json:"HasMore"`
		Items   []zhihu.Item `json:"Items"`
	}{Items: []zhihu.Item{}}}

	result, err := Analyze(context.Background(), "600519", "贵州茅台", fakeSearcher{resp: resp}, ds)
	if err != nil {
		t.Fatalf("空结果时应降级而非返回 error: %v", err)
	}
	if !result.Degraded {
		t.Errorf("空结果时应 Degraded=true")
	}
	if result.Score != nil {
		t.Errorf("降级时 Score 应为 nil")
	}
}
