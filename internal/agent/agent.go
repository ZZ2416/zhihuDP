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
	ResolveStock func(ctx context.Context, query string) (*types.StockInfo, error)
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
				return "", err
			}
			_ = sink(types.Event{Type: "stock", Data: info})
			b, _ := json.Marshal(info)
			return string(b), nil
		})
	if err != nil {
		return nil, fmt.Errorf("构建 resolve_stock 工具失败: %w", err)
	}

	cm, err := newDeepSeekModel(ctx, deps.DeepSeek())
	if err != nil {
		return nil, fmt.Errorf("构建 DeepSeek 模型失败: %w", err)
	}

	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name: "stock-analysis-agent",
		Instruction: `你是股票基本面分析助手。流程固定：
1) 必须先调用 resolve_stock 识别股票；若返回 found=false，直接回复其中的 message（如「未找到该股票，请检查名称或代码」），不要继续调用其他工具。
严格约束：不推荐买卖、不评股价、不给目标价，不用「概率/胜率/预测」措辞；不编造数据。`,
		Model: cm,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: []tool.BaseTool{resolveTool},
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
