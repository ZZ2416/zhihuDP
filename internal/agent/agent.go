// Package agent controller 层：业务编排（eino ADK ChatModelAgent ReAct 循环 + 工具 + 事件分发）
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"zhihudp/internal/compliance"
	"zhihudp/internal/config"
	"zhihudp/internal/types"
)

// Deps agent 依赖（由 cmd/server 组装注入，避免全局状态）
type Deps struct {
	ResolveStock     func(ctx context.Context, query string) (*types.StockInfo, error)
	AnalyzeSentiment func(ctx context.Context, code, name string) (*types.SentimentResult, error)
	FundamentalScore func(ctx context.Context, code, market string) (*types.FundamentalResult, error)
	// DeepSeek 配置 getter：每次调用取最新（支持密钥热更新）
	DeepSeek func() config.DeepSeekConfig
}

// RunAnalysis 运行 agent，把事件分发到 sink（SSE 转发 / CLI 打印）
func RunAnalysis(ctx context.Context, query string, deps Deps, sink func(types.Event) error) error {
	agent, err := newChatModelAgent(ctx, deps, sink)
	if err != nil {
		return err
	}

	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: agent, EnableStreaming: true})
	iter := runner.Query(ctx, query)

	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			if err := sink(types.Event{Type: "error", Data: map[string]string{"message": ev.Err.Error()}}); err != nil {
				return err
			}
			continue
		}
		if ev.Output == nil || ev.Output.MessageOutput == nil {
			continue
		}
		mv := ev.Output.MessageOutput
		if mv.Role != schema.Assistant {
			continue // 工具事件已在工具闭包内通过 sink 直发
		}
		if mv.IsStreaming && mv.MessageStream != nil {
			// 内层函数保证任何返回路径都执行 Close，避免 sink 出错时泄漏流
			err := func() error {
				for {
					chunk, err := mv.MessageStream.Recv()
					if errors.Is(err, io.EOF) {
						return nil
					}
					if err != nil {
						_ = sink(types.Event{Type: "error", Data: map[string]string{"message": err.Error()}})
						return nil
					}
					if text := compliance.Filter(chunk.Content); text != "" {
						if err := sink(types.Event{Type: "delta", Data: map[string]string{"text": text}}); err != nil {
							return err
						}
					}
				}
			}()
			mv.MessageStream.Close()
			if err != nil {
				return err
			}
		} else {
			msg, err := mv.GetMessage()
			if err != nil {
				continue
			}
			if text := compliance.Filter(msg.Content); text != "" {
				if err := sink(types.Event{Type: "delta", Data: map[string]string{"text": text}}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// newChatModelAgent 组装 ADK ChatModelAgent（ReAct 循环 + 2 个自定义工具）
func newChatModelAgent(ctx context.Context, deps Deps, sink func(types.Event) error) (*adk.ChatModelAgent, error) {
	resolveTool, err := utils.InferTool("resolve_stock",
		"将股票名称或代码解析为股票信息（代码、名称、市场）。必须先调用本工具识别股票；未找到时返回 found=false。",
		func(ctx context.Context, req *struct {
			Query string `json:"query" jsonschema_description:"股票名称或代码，如 茅台 / 600519"`
		}) (string, error) {
			info, err := deps.ResolveStock(ctx, req.Query)
			if err != nil {
				// 业务降级：未找到不是错误，返回结构化标记让 agent 优雅回复（避免原始错误冒泡给用户）
				if errors.Is(err, types.ErrStockNotFound) {
					b, _ := json.Marshal(map[string]any{
						"found":   false,
						"message": "未找到该股票，请检查名称或代码",
					})
					return string(b), nil
				}
				// 非 NotFound 错误（网络抖动等）：降级返回，避免终止整个分析
				b, _ := json.Marshal(map[string]any{
					"found":    false,
					"degraded": true,
					"err_msg":  "股票识别服务暂时不可用，请稍后重试",
					"message":  "股票识别服务暂时不可用，请稍后重试",
				})
				return string(b), nil
			}
			_ = sink(types.Event{Type: "stock", Data: info})
			b, _ := json.Marshal(info)
			return string(b), nil
		})
	if err != nil {
		return nil, fmt.Errorf("构建 resolve_stock 工具失败: %w", err)
	}

	sentTool, err := utils.InferTool("analyze_sentiment",
		"分析股票在知乎的讨论情绪，返回热度、多空占比、参考强度分、代表观点。",
		func(ctx context.Context, req *struct {
			Code string `json:"code" jsonschema_description:"股票代码，如 600519"`
			Name string `json:"name" jsonschema_description:"股票名称，如 贵州茅台"`
		}) (string, error) {
			result, err := deps.AnalyzeSentiment(ctx, req.Code, req.Name)
			if err != nil {
				b, _ := json.Marshal(map[string]any{"degraded": true, "err_msg": "情绪数据暂时不可用，请稍后重试"})
				return string(b), nil
			}
			_ = sink(types.Event{Type: "sentiment", Data: result})
			b, _ := json.Marshal(result)
			return string(b), nil
		})
	if err != nil {
		return nil, fmt.Errorf("构建 analyze_sentiment 工具失败: %w", err)
	}

	fundTool, err := utils.InferTool("analyze_fundamental",
		"分析股票基本面，返回四维评分（盈利/成长/财务健康/估值）、财务指标与估值数据。",
		func(ctx context.Context, req *struct {
			Code   string `json:"code" jsonschema_description:"股票代码，如 600519"`
			Market string `json:"market" jsonschema_description:"市场，如 沪A/深A"`
		}) (string, error) {
			result, err := deps.FundamentalScore(ctx, req.Code, req.Market)
			if err != nil {
				// 降级：返回 JSON，LLM 如实说明「财务数据不可用」，避免前端裸错误
				b, _ := json.Marshal(map[string]any{"degraded": true, "err_msg": "财务数据暂时不可用，请稍后重试"})
				return string(b), nil
			}
			_ = sink(types.Event{Type: "fundamental", Data: result})
			b, _ := json.Marshal(result)
			return string(b), nil
		})
	if err != nil {
		return nil, fmt.Errorf("构建 analyze_fundamental 工具失败: %w", err)
	}

	cm, err := newDeepSeekModel(ctx, deps.DeepSeek())
	if err != nil {
		return nil, fmt.Errorf("构建 DeepSeek 模型失败: %w", err)
	}

	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name: "stock-analysis-agent",
		Instruction: `你是股票分析助手。流程固定：
0) 禁止输出任何过程性/思考性文字（如「我来分析」「先识别股票」「正在获取数据」等），直接调用工具，最终只输出解读正文；
1) 必须先调用 resolve_stock 识别股票；若返回 found=false，直接回复其中的 message（如「未找到该股票，请检查名称或代码」），不要继续调用其他工具；
2) 再调用 analyze_sentiment 获取知乎情绪数据（热度/多空/参考强度/代表观点）；
3) 再调用 analyze_fundamental 获取四维评分、财务指标与估值；
4) 基于返回的 JSON 撰写基本面解读，分五段：
   一、综合质地（引用 score.total 总分与 grade 定性）；
   二、盈利能力（ROE/毛利率/净利率水平与趋势）；
   三、成长性（营收/净利同比与历史趋势）；
   四、财务健康（资产负债率/经营现金流）；
   五、估值与风险（PE 历史分位、PB；注明分位仅供参考，未做行业对比）。
若返回 degraded=true 或 err_msg 非空，如实说明数据不足，不要编造。
严格约束：不推荐买卖、不评股价、不给目标价，不用「概率/胜率/预测」措辞；不编造返回 JSON 中不存在的数值；文末注明「数据来源东方财富/腾讯，不构成投资建议」。`,
		Model: cm,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: []tool.BaseTool{resolveTool, sentTool, fundTool},
			},
		},
		MaxIterations: 5,
	})
	if err != nil {
		return nil, fmt.Errorf("构建 agent 失败: %w", err)
	}
	return agent, nil
}

func newDeepSeekModel(ctx context.Context, ds config.DeepSeekConfig) (*deepseek.ChatModel, error) {
	return deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
		APIKey:  ds.APIKey,
		Model:   "deepseek-chat",
		BaseURL: ds.BaseURL,
		Timeout: time.Duration(ds.Timeout),
	})
}
