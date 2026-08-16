package main

import (
	"context"
	"strings"
	"testing"
)

func TestComputeStrength(t *testing.T) {
	cases := []struct {
		name string
		r    Ratio
		sample, heat int
		wantMin, wantMax int
	}{
		{"一致性强+样本足", Ratio{Bull: 0.9, Bear: 0.05}, 50, 40, 7, 10},
		{"分歧大+样本足", Ratio{Bull: 0.5, Bear: 0.45}, 50, 40, 4, 7},
		{"一致性强+样本少", Ratio{Bull: 0.9, Bear: 0.05}, 3, 5, 4, 8},
	}
	for _, c := range cases {
		got := ComputeStrength(c.r, c.sample, c.heat)
		if got < c.wantMin || got > c.wantMax {
			t.Errorf("%s: ComputeStrength=%d 期望 [%d,%d]", c.name, got, c.wantMin, c.wantMax)
		}
	}
	// 边界钳制
	if s := ComputeStrength(Ratio{}, 0, 0); s < 1 || s > 10 {
		t.Errorf("边界钳制失败: %d", s)
	}
	if s := ComputeStrength(Ratio{Bull: 1, Bear: 0}, 1000, 1000); s < 1 || s > 10 {
		t.Errorf("上限钳制失败: %d", s)
	}
}

func TestFilter(t *testing.T) {
	// 流式块：含禁用词整块丢弃
	if got := Filter("这只股票建议买入"); got != "" {
		t.Errorf("Filter 应丢弃含禁用词的块: %q", got)
	}
	if got := Filter("讨论热度较高"); got != "讨论热度较高" {
		t.Errorf("Filter 不应误伤正常文本: %q", got)
	}
}

func TestFilterFinal(t *testing.T) {
	in := "情绪面看多观点占比较高。建议买入，风险自担。风险提示：注意波动。"
	got := FilterFinal(in)
	if strings.Contains(got, "建议买入") {
		t.Errorf("FilterFinal 应剔除含禁用词的句子: %q", got)
	}
	if !strings.Contains(got, "情绪面看多观点占比较高") {
		t.Errorf("FilterFinal 不应误删正常句子: %q", got)
	}
	// 全部被剔除 → 兜底文案
	if got := FilterFinal("建议买入。"); got == "" {
		t.Errorf("FilterFinal 全剔除时应返回兜底文案")
	}
}

func TestAnalyzeSentiment_NoKey_Degraded(t *testing.T) {
	// 无 deepseek key：降级而非报错（业务规则 §4.5）
	cfg = &Config{}
	result, err := AnalyzeSentiment(context.Background(), "600519", "贵州茅台")
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
