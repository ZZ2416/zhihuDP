// 财报 AI 解析：财务分析 prompt + 流式 LLM + 合规过滤（只解读，不荐股）
package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"

	"zhihudp/internal/compliance"
	"zhihudp/internal/types"
)

// financeSystemPrompt 财务解析人设与约束（数据仅来自注入指标，不编造）
const financeSystemPrompt = `你是专业的财务分析助手，擅长把枯燥的财务数字讲清楚。基于下方提供的公司财务指标（最近5年年报+最新报告期）输出解析，分五段：
1. 盈利能力与质量：毛利率、净利率、ROE 的水平与趋势；
2. 成长性：营收/净利润同比变化与 5 年趋势；
3. 财务健康：资产负债率、经营现金流状况；
4. 值得关注的指标：同比波动较大（如超 20%）的项目重点说明；
5. 风险提示。
硬性约束：
- 只基于给定数据，不编造任何数值；
- 不推荐买卖、不评价股价、不给目标价，不用「概率」「预测」等措辞；
- 数据不足时如实说明「看山这边没有这项数据」；
- 最后一行注明：数据来源东方财富，不构成投资建议。`

// AnalyzeFinance 运行一轮财报 AI 解析，增量经 sink 转发（delta → 收尾 FilterFinal）
func AnalyzeFinance(ctx context.Context, code, name string, indicators []types.FinancialIndicator, deps Deps, sink func(types.Event) error) error {
	cm, err := newDeepSeekModel(ctx, deps.DeepSeek())
	if err != nil {
		return fmt.Errorf("构建 DeepSeek 模型失败: %w", err)
	}

	msgs := []*schema.Message{
		schema.SystemMessage(financeSystemPrompt + "\n\n" + formatFinanceFacts(code, name, indicators)),
		schema.UserMessage("请解析这家公司的财务情况。"),
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
			break
		}
		text := compliance.Filter(msg.Content) // 流式增量过滤
		if text == "" {
			continue
		}
		final.WriteString(text)
		if err := sink(types.Event{Type: "delta", Data: map[string]string{"text": text}}); err != nil {
			return err
		}
	}
	// 收尾整段过滤
	finalText := compliance.FilterFinal(final.String())
	if finalText != final.String() {
		if err := sink(types.Event{Type: "delta", Data: map[string]string{"text": finalText}}); err != nil {
			return err
		}
	}
	return nil
}

// formatFinanceFacts 指标 → 注入文本（表格形式）
func formatFinanceFacts(code, name string, items []types.FinancialIndicator) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("\n===== 公司财务指标 =====\n公司：%s（%s）\n", name, code))
	b.WriteString("金额单位：亿元；比率单位：%；经营现金流/营收为比值。\n\n")
	b.WriteString("| 报告期 | 营业总收入 | 营收同比 | 归母净利润 | 净利同比 | 每股收益 | ROE | 毛利率 | 净利率 | 资产负债率 | 经营现金流/营收 |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, it := range items {
		b.WriteString(fmt.Sprintf("| %s | %.2f | %+.2f | %.2f | %+.2f | %.2f | %.2f | %.2f | %.2f | %.2f | %.2f |\n",
			it.ReportDate, it.Revenue, it.RevenueYoY, it.NetProfit, it.NetProfitYoY,
			it.EPS, it.ROE, it.GrossMargin, it.NetMargin, it.DebtRatio, it.CashFlowToRev))
	}
	b.WriteString("===== 指标结束 =====\n")
	return b.String()
}
