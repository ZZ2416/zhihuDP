// 情绪 AI 解读：注入情绪数据（热度/多空/参考强度/代表观点）→ 3 段解读，流式 + 合规
package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino/schema"

	"zhihudp/internal/compliance"
	"zhihudp/internal/types"
)

// emotionSystemPrompt 情绪解读约束
const emotionSystemPrompt = `你是社区情绪分析助手。基于下方提供的股票知乎讨论情绪数据，输出解读，分三段：
0. 禁止输出任何过程性/思考性文字（如「我来解读」「先看数据」等），直接输出解读正文；
1. 情绪面总结：多空占比、讨论热度、参考强度；
2. 值得关注的讨论点：引用代表观点（标题+链接）；
3. 风险提示。
硬性约束：
- 只基于给定数据，不编造观点与数值；
- 不推荐买卖、不评股价，不用「概率/预测」措辞；
- 数据降级（degraded=true）时如实说明「讨论较少或搜索受限」，不臆测；
- 最后注明：数据来源知乎公开讨论，不构成投资建议。`

// AnalyzeEmotion 情绪解读：增量经 sink 转发（delta → done/error）
func AnalyzeEmotion(ctx context.Context, name string, s *types.SentimentResult, deps Deps, sink func(types.Event) error) error {
	cm, err := newDeepSeekModel(ctx, deps.DeepSeek())
	if err != nil {
		return fmt.Errorf("构建 DeepSeek 模型失败: %w", err)
	}
	msgs := []*schema.Message{
		schema.SystemMessage(emotionSystemPrompt + "\n\n" + formatEmotionFacts(name, s)),
		schema.UserMessage("请解读这只股票的社区情绪。"),
	}
	stream, err := cm.Stream(ctx, msgs)
	if err != nil {
		return fmt.Errorf("DeepSeek 流式请求失败: %w", err)
	}
	defer stream.Close()

	var final strings.Builder
	for {
		msg, err := stream.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) && ctx.Err() == nil {
				_ = sink(types.Event{Type: "error", Data: map[string]string{"message": "流式输出中断: " + err.Error()}})
			}
			break
		}
		text := compliance.Filter(msg.Content)
		if text == "" {
			continue
		}
		final.WriteString(text)
		if err := sink(types.Event{Type: "delta", Data: map[string]string{"text": text}}); err != nil {
			return err
		}
	}
	_ = compliance.FilterFinal(final.String())
	return nil
}

// formatEmotionFacts 情绪数据 → 注入文本
func formatEmotionFacts(name string, s *types.SentimentResult) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("\n===== 股票情绪数据 =====\n股票：%s\n", name))
	if s == nil {
		b.WriteString("无情绪数据。\n===== 数据结束 =====\n")
		return b.String()
	}
	if s.Degraded {
		b.WriteString("（数据降级：" + s.ErrMsg + "）\n")
	}
	b.WriteString(fmt.Sprintf("讨论热度 %d，样本 %d；看多 %.0f%%，看空 %.0f%%，中性 %.0f%%",
		s.Heat, s.Sample, s.Ratio.Bull*100, s.Ratio.Bear*100, s.Ratio.Neutral*100))
	if s.Score != nil {
		b.WriteString(fmt.Sprintf("；参考强度 %d/10", *s.Score))
	}
	b.WriteString("\n代表观点：\n")
	for i, it := range s.Items {
		if i >= 5 {
			break
		}
		b.WriteString(fmt.Sprintf("%d. %s（%s）\n", i+1, it.Title, it.Url))
	}
	b.WriteString("===== 数据结束 =====\n")
	return b.String()
}
