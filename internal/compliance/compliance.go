// Package compliance 合规措辞过滤（business-design §5 红线 + product-design §8 禁用词表）
package compliance

import "strings"

// bannedWords 禁用词
var bannedWords = []string{
	"买入", "卖出", "持有", "建议建仓", "建议加仓", "建议减仓", "建议买入", "建议卖出",
	"可以上车", "赶紧买", "清仓", "满仓", "建仓", "抄底", "追高", "买进", "抛售", "减仓",
	"概率", "预测", "预判", "推荐", "荐股",
}

// Filter 流式增量过滤：块内命中禁用词则整块丢弃（块小影响小；agent 已受 prompt 约束）
func Filter(text string) string {
	if text == "" {
		return ""
	}
	if containsAny(text, bannedWords) {
		return ""
	}
	return text
}

// FilterFinal 完整文本过滤：按句剔除含禁用词的句子（CLI/收尾用）
func FilterFinal(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	if !containsAny(text, bannedWords) {
		return text
	}
	var kept []string
	cur := ""
	for _, r := range text {
		cur += string(r)
		if r == '。' || r == '！' || r == '？' || r == '\n' {
			if !containsAny(cur, bannedWords) {
				kept = append(kept, cur)
			}
			cur = ""
		}
	}
	if cur != "" && !containsAny(cur, bannedWords) {
		kept = append(kept, cur)
	}
	out := strings.Join(kept, "")
	if strings.TrimSpace(out) == "" {
		return "当前分析内容暂不可用，请查看上方情绪面板数据。"
	}
	return out
}

func containsAny(s string, words []string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}
