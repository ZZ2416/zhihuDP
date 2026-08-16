// 二期「AI 看山」追问对话：人设 system prompt + 流式 LLM + 合规过滤
package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"

	"zhihudp/internal/compliance"
	"zhihudp/internal/types"
)

// kanshanSystemPrompt 「AI 看山」人设 + 合规约束（skill: ai_kanshan）
const kanshanSystemPrompt = `你是「AI 看山」，知乎吉祥物刘看山（北极狐）的化身，擅长以温和、拟人的口吻和用户聊股票投资话题。
身份与口吻：
- 自称「看山」；语气温和、专业克制，偶尔带一点俏皮，但绝不开玩笑误导；
- 只基于下方提供的上下文事实回答，不编造数据；上下文没有的信息，明确说「看山这边没有这个数据」。
硬性约束（必须遵守）：
1. 不提供任何投资决策：不推荐、不暗示买卖，不给目标价，不用「概率」「胜率」「预测」等措辞；
2. 用户问「该买吗/该卖吗/能抄底吗/要不要加仓」时，先温和说明「看山是信息工具，不给出买卖建议」，再转向分析方法、关注维度、风险提示；
3. 回答控制在 3-6 句话，多用「看山觉得」「看山注意到」；
4. 涉及合规词汇时用「操作」「调整」「关注」等中性词替代。`

// Chat 运行一轮「AI 看山」对话，增量经 sink 转发（delta → 收尾 FilterFinal），
// 全程合规过滤；返回最终文本由 chat.Service 通过 sink 累积。
func Chat(ctx context.Context, facts types.ChatFacts, history []types.ChatMessage, message string, deps Deps, sink func(types.Event) error) error {
	cm, err := newDeepSeekModel(ctx, deps.DeepSeek())
	if err != nil {
		return fmt.Errorf("构建 DeepSeek 模型失败: %w", err)
	}

	// 消息序列：system（人设+事实）→ 历史 → 新提问
	messages := []*schema.Message{
		schema.SystemMessage(kanshanSystemPrompt + "\n\n" + formatFacts(facts)),
	}
	for _, h := range history {
		if strings.TrimSpace(h.Content) == "" {
			continue
		}
		if h.Role == "user" {
			messages = append(messages, schema.UserMessage(h.Content))
		} else {
			messages = append(messages, schema.AssistantMessage(h.Content, nil))
		}
	}
	messages = append(messages, schema.UserMessage(message))

	stream, err := cm.Stream(ctx, messages)
	if err != nil {
		return fmt.Errorf("DeepSeek 流式请求失败: %w", err)
	}
	defer stream.Close()

	var final strings.Builder
	for {
		msg, err := stream.Recv()
		if err != nil {
			break // io.EOF 或上下文取消：正常结束
		}
		text := msg.Content
		if text == "" {
			continue
		}
		text = compliance.Filter(text) // 流式增量过滤：命中禁用词整块丢弃
		if text == "" {
			continue
		}
		final.WriteString(text)
		if err := sink(types.Event{Type: "delta", Data: map[string]string{"text": text}}); err != nil {
			return err
		}
	}

	// 收尾：整段句级过滤，若被删空给兜底文案
	finalText := compliance.FilterFinal(final.String())
	if finalText != final.String() {
		if err := sink(types.Event{Type: "delta", Data: map[string]string{"text": finalText}}); err != nil {
			return err
		}
	}
	return nil
}

// formatFacts 上下文事实 → system prompt 附文
func formatFacts(f types.ChatFacts) string {
	var b strings.Builder
	b.WriteString("\n===== 当前上下文事实 =====\n")
	if f.StockName != "" || f.StockCode != "" {
		b.WriteString(fmt.Sprintf("股票：%s（%s，%s）\n", f.StockName, f.StockCode, f.Market))
	}
	if f.Quote != "" {
		b.WriteString("行情快照：" + f.Quote + "\n")
	}
	if f.Sentiment != "" {
		b.WriteString("知乎情绪：" + f.Sentiment + "\n")
	}
	if f.Knowledge != "" {
		b.WriteString("知识库片段：\n" + f.Knowledge)
	}
	if f.PrevAnalysis != "" {
		b.WriteString("\n此前 AI 分析（引用其结论时保持一致）：\n" + f.PrevAnalysis)
	}
	b.WriteString("\n===== 上下文结束 =====\n")
	return b.String()
}
