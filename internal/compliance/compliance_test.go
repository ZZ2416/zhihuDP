package compliance

import (
	"strings"
	"testing"
)

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
