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
	DeepSeek         config.DeepSeekConfig
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
		"将股票名称或代码解析为股票信息（代码、名称、市场）。必须先调用本工具识别股票。",
		func(ctx context.Context, req *struct {
			Query string `json:"query" jsonschema_description:"股票名称或代码，如 茅台 / 600519"`
		}) (string, error) {
			info, err := deps.ResolveStock(ctx, req.Query)
			if err != nil {
				return "", err
			}
			_ = sink(types.Event{Type: "stock", Data: info})
			b, _ := json.Marshal(info)
			return string(b), nil
		})
	if err != nil {
		return nil, fmt.Errorf("构建 resolve_stock 工具失败: %w", err)
	}

	sentimentTool, err := utils.InferTool("analyze_sentiment",
		"分析股票在知乎的讨论情绪，返回热度、多空占比、参考强度分、代表观点。",
		func(ctx context.Context, req *struct {
			Code string `json:"code" jsonschema_description:"股票代码，如 600519"`
			Name string `json:"name" jsonschema_description:"股票名称，如 贵州茅台"`
		}) (string, error) {
			result, err := deps.AnalyzeSentiment(ctx, req.Code, req.Name)
			if err != nil {
				return "", err
			}
			_ = sink(types.Event{Type: "sentiment", Data: result})
			b, _ := json.Marshal(result)
			return string(b), nil
		})
	if err != nil {
		return nil, fmt.Errorf("构建 analyze_sentiment 工具失败: %w", err)
	}

	cm, err := newDeepSeekModel(ctx, deps.DeepSeek)
	if err != nil {
		return nil, fmt.Errorf("构建 DeepSeek 模型失败: %w", err)
	}

	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name: "zhihu-stock-sentiment-agent",
		Instruction: `你是知乎股票情绪分析助手。流程固定：
1) 必须先调用 resolve_stock 识别股票；
2) 再调用 analyze_sentiment 获取情绪数据；
3) 基于返回的 JSON 撰写分析面板，分三段：情绪面总结、值得关注的讨论点、风险提示。
若返回 degraded=true 或 err_msg 非空，如实说明数据不足或无法检索，不要编造。
严格约束：不得出现买入/卖出/持有/建议/概率/预测/推荐/荐股等投资建议措辞；不得编造返回 JSON 中不存在的观点；文末可列出来源（标题+链接）。`,
		Model: cm,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: []tool.BaseTool{resolveTool, sentimentTool},
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
