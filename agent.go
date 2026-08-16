package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// eventSinkKey 通过 context 把事件回调注入工具层（工具内直接发结构化事件）
type eventSinkKey struct{}

func withEventSink(ctx context.Context, sink func(Event) error) context.Context {
	return context.WithValue(ctx, eventSinkKey{}, sink)
}

func emitEvent(ctx context.Context, ev Event) {
	if s, ok := ctx.Value(eventSinkKey{}).(func(Event) error); ok {
		_ = s(ev)
	}
}

// newAgent 组装 eino ADK ChatModelAgent（ReAct 循环 + 2 个自定义工具）
func newAgent(ctx context.Context) (*adk.ChatModelAgent, error) {
	resolveTool, err := utils.InferTool("resolve_stock",
		"将股票名称或代码解析为股票信息（代码、名称、市场）。必须先调用本工具识别股票。",
		func(ctx context.Context, req *struct {
			Query string `json:"query" jsonschema_description:"股票名称或代码，如 茅台 / 600519"`
		}) (string, error) {
			info, err := ResolveStock(ctx, req.Query)
			if err != nil {
				return "", err
			}
			emitEvent(ctx, Event{Type: "stock", Data: info})
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
			result, err := AnalyzeSentiment(ctx, req.Code, req.Name)
			if err != nil {
				return "", err
			}
			emitEvent(ctx, Event{Type: "sentiment", Data: result})
			b, _ := json.Marshal(result)
			return string(b), nil
		})
	if err != nil {
		return nil, fmt.Errorf("构建 analyze_sentiment 工具失败: %w", err)
	}

	cm, err := newDeepSeekModel(ctx)
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

// runAnalysis 运行 agent 并把事件分发到 sink（SSE 转发 / CLI 打印）
func runAnalysis(ctx context.Context, stockQuery string, sink func(Event) error) error {
	agent, err := newAgent(ctx)
	if err != nil {
		return err
	}
	ctx = withEventSink(ctx, sink)

	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: agent, EnableStreaming: true})
	iter := runner.Query(ctx, stockQuery)

	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			if err := sink(Event{Type: "error", Data: map[string]string{"message": ev.Err.Error()}}); err != nil {
				return err
			}
			continue
		}
		if ev.Output == nil || ev.Output.MessageOutput == nil {
			continue
		}
		mv := ev.Output.MessageOutput
		if mv.Role != schema.Assistant {
			continue // 工具事件已在工具内部通过 emitEvent 发送
		}
		if mv.IsStreaming && mv.MessageStream != nil {
			for {
				chunk, err := mv.MessageStream.Recv()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					_ = sink(Event{Type: "error", Data: map[string]string{"message": err.Error()}})
					break
				}
				if text := Filter(chunk.Content); text != "" {
					if err := sink(Event{Type: "delta", Data: map[string]string{"text": text}}); err != nil {
						return err
					}
				}
			}
			mv.MessageStream.Close()
		} else {
			msg, err := mv.GetMessage()
			if err != nil {
				continue
			}
			if text := Filter(msg.Content); text != "" {
				if err := sink(Event{Type: "delta", Data: map[string]string{"text": text}}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
