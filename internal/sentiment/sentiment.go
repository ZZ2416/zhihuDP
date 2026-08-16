// Package sentiment 情绪分析：知乎搜索 → LLM 批量情感分类 → 规则强度分
package sentiment

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino/schema"

	"zhihudp/internal/config"
	"zhihudp/internal/types"
	"zhihudp/internal/zhihu"
)

// Analyze 编排：知乎搜索 → LLM 批量情感分类 → 规则强度分。
// 任何环节失败都降级（Degraded=true）而非返回 error，让 agent 走降级文案。
func Analyze(ctx context.Context, code, name string, zh *zhihu.Client, ds config.DeepSeekConfig) (*types.SentimentResult, error) {
	result := &types.SentimentResult{Code: code, Name: name, Items: []types.ViewItem{}}

	if ds.APIKey == "" {
		result.Degraded = true
		result.ErrMsg = "未配置 deepseek.api_key，无法进行情感分类"
		return result, nil
	}

	// 1) 知乎搜索（近 30 天讨论，取 10 条）
	sr, err := zh.Search(ctx, name, 10)
	if err != nil {
		result.Degraded = true
		result.ErrMsg = "知乎搜索失败：" + err.Error()
		return result, nil
	}
	if len(sr.Data.Items) == 0 {
		result.Degraded = true
		result.ErrMsg = "该股近 30 天知乎讨论较少，暂未生成情绪画像"
		return result, nil
	}

	// 2) LLM 批量情感分类
	labels, err := classifyPosts(ctx, name, sr.Data.Items, ds)
	if err != nil {
		result.Degraded = true
		result.ErrMsg = "情感分类失败：" + err.Error()
		return result, nil
	}

	// 3) 组装结构化结果
	result.Heat = len(sr.Data.Items)
	result.Sample = len(labels)
	counts := map[string]int{}
	items := make([]types.ViewItem, 0, len(sr.Data.Items))
	for i, it := range sr.Data.Items {
		label := "neutral"
		if i < len(labels) && labels[i] != "" {
			label = labels[i]
		}
		counts[label]++
		items = append(items, types.ViewItem{
			Title:     it.Title,
			Url:       it.Url,
			Author:    it.AuthorName,
			VoteUp:    it.VoteUpCount,
			Excerpt:   truncate(it.ContentText, 120),
			Sentiment: label,
		})
	}
	total := float64(len(labels))
	result.Ratio = types.Ratio{
		Bull:    round3(float64(counts["bull"]) / total),
		Bear:    round3(float64(counts["bear"]) / total),
		Neutral: round3(float64(counts["neutral"]) / total),
	}

	// 代表观点：互动量降序取前 5
	sort.SliceStable(items, func(i, j int) bool { return items[i].VoteUp > items[j].VoteUp })
	if len(items) > 5 {
		items = items[:5]
	}
	result.Items = items

	// 4) 规则强度分（样本不足不展示）
	if result.Sample >= 5 {
		score := ComputeStrength(result.Ratio, result.Sample, result.Heat)
		result.Score = &score
	}
	return result, nil
}

// classifyPosts LLM 批量情感分类：一次调用分类 N 条，返回与输入对齐的标签数组
func classifyPosts(ctx context.Context, stockName string, items []zhihu.Item, ds config.DeepSeekConfig) ([]string, error) {
	if len(items) == 0 {
		return []string{}, nil
	}
	cm, err := newDeepSeekModel(ctx, ds)
	if err != nil {
		return nil, err
	}

	type itemIn struct {
		ID      int    `json:"id"`
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	inputs := make([]itemIn, 0, len(items))
	for i, it := range items {
		inputs = append(inputs, itemIn{ID: i, Title: it.Title, Content: truncate(it.ContentText, 120)})
	}
	inJSON, _ := json.Marshal(inputs)

	prompt := fmt.Sprintf(`你是股票讨论情感分析器。分析对象：%s。
请判断下面每条讨论对该股的情绪倾向，只输出 JSON 数组（不要任何其他文字、不要 markdown 代码块）。
格式：[{"id": 编号, "sentiment": "bull"}]
sentiment 取值：bull=看多/乐观，bear=看空/悲观，neutral=中性/无明确倾向。
讨论列表：
%s`, stockName, string(inJSON))

	ctx2, cancel := context.WithTimeout(ctx, 60*time.Second) // 分类调用独立超时 60s
	defer cancel()
	out, err := cm.Generate(ctx2, []*schema.Message{schema.UserMessage(prompt)})
	if err != nil {
		return nil, err
	}

	var outs []struct {
		ID        int    `json:"id"`
		Sentiment string `json:"sentiment"`
	}
	content := trimCodeFence(out.Content)
	if err := json.Unmarshal([]byte(content), &outs); err != nil {
		return nil, fmt.Errorf("分类结果 JSON 解析失败: %w", err)
	}

	labels := make([]string, len(items))
	for _, o := range outs {
		if o.ID < 0 || o.ID >= len(labels) {
			continue
		}
		switch o.Sentiment {
		case "bull", "bear", "neutral":
			labels[o.ID] = o.Sentiment
		default:
			labels[o.ID] = "neutral"
		}
	}
	return labels, nil
}

// newDeepSeekModel 创建 DeepSeek ChatModel（agent 与分类共用）
func newDeepSeekModel(ctx context.Context, ds config.DeepSeekConfig) (*deepseek.ChatModel, error) {
	return deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
		APIKey:  ds.APIKey,
		Model:   "deepseek-chat",
		BaseURL: ds.BaseURL,
		Timeout: time.Duration(ds.Timeout),
	})
}

// ComputeStrength 规则计算参考强度 1-10（非概率，可解释可复现）。
// 公式：1 + 6*多空一致性 + log10(1+样本)/2 + 热度修正
//   - 多空一致性（max(bull,bear)）主导：五五开≈4-5 分，强一致≈7-9 分
//   - 样本量、热度仅作小幅修正，避免样本多但分歧大的场景虚高
func ComputeStrength(r types.Ratio, sample, heat int) int {
	dominance := math.Max(r.Bull, r.Bear)
	s := 1 + 6*dominance + math.Log10(1+float64(sample))/2 + math.Min(float64(heat), 50)/100
	score := int(math.Round(s))
	if score < 1 {
		score = 1
	}
	if score > 10 {
		score = 10
	}
	return score
}

func round3(f float64) float64 {
	return math.Round(f*1000) / 1000
}

// trimCodeFence 兼容模型偶尔输出 ```json ... ``` 包裹
func trimCodeFence(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
