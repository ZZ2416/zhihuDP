package sentiment

import (
	"context"
	"testing"

	"zhihudp/internal/config"
	"zhihudp/internal/types"
	"zhihudp/internal/zhihu"
)

func TestComputeStrength(t *testing.T) {
	cases := []struct {
		name          string
		r             types.Ratio
		sample, heat  int
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
	zhClient := zhihu.New(config.ZhihuConfig{})
	result, err := Analyze(context.Background(), "600519", "贵州茅台", zhClient, config.DeepSeekConfig{})
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
