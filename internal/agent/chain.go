// 产业链 AI 生成：LLM 输出结构化 JSON（环节/边/厂商），严格 JSON 约束 + 失败重试 1 次
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"

	"zhihudp/internal/types"
)

// chainPrompt 产业链生成约束（只输出 JSON）
const chainPrompt = `你是产业研究助手。基于股票「%s（%s）」生成其所在产业链的结构化数据，输出严格 JSON（不要输出任何其他文字）：
{
  "industry": "产业链名称（如 白酒产业链）",
  "nodes": [{"id":"n1","name":"环节名（含 上游/中游/下游 前缀，如 上游-粮食种植）","stage":"上游|中游|下游","desc":"环节说明，20字以内"}],
  "edges": [{"from":"n1","to":"n2"}],
  "companies": {"n1":[{"code":"600519","name":"贵州茅台"}], "n2":[...]}
}
要求：
1. 环节（nodes）6-10 个，覆盖 上游/中游/下游 三阶段；
2. 每个环节 3-6 家同类型代表厂商，必须是真实存在的 A 股上市公司（代码+名称准确）；
3. edges 表达环节间上下游关系，沿 上游→中游→下游 方向；
4. companies 的键必须对应 nodes 的 id；
5. 只输出一个 JSON 对象，无 markdown 代码块、无注释。`

// GenerateChain LLM 生成产业链 JSON；解析失败重试 1 次
func GenerateChain(ctx context.Context, name, code string, deps Deps) (*types.ChainResult, error) {
	cm, err := newDeepSeekModel(ctx, deps.DeepSeek())
	if err != nil {
		return nil, fmt.Errorf("构建 DeepSeek 模型失败: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		prompt := fmt.Sprintf(chainPrompt, name, code)
		if attempt > 0 {
			prompt += "\n（上次输出不是合法 JSON，请重新输出，必须是单个 JSON 对象）"
		}
		msg, err := cm.Generate(ctx, []*schema.Message{schema.UserMessage(prompt)})
		if err != nil {
			lastErr = fmt.Errorf("LLM 生成失败: %w", err)
			continue
		}
		res, err := parseChain(msg.Content)
		if err != nil {
			lastErr = err
			continue
		}
		if res != nil {
			return res, nil
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("生成结果为空")
	}
	return nil, fmt.Errorf("产业链生成失败（重试后仍失败）: %w", lastErr)
}

// parseChain 解析并校验 LLM 输出的产业链 JSON
func parseChain(raw string) (*types.ChainResult, error) {
	// 剥离可能的 markdown 代码块围栏
	text := strings.TrimSpace(raw)
	if strings.HasPrefix(text, "```") {
		if i := strings.Index(text, "\n"); i >= 0 {
			text = strings.TrimSpace(text[i+1:])
		}
		if i := strings.LastIndex(text, "```"); i >= 0 {
			text = strings.TrimSpace(text[:i])
		}
	}
	var res types.ChainResult
	if err := json.Unmarshal([]byte(text), &res); err != nil {
		return nil, fmt.Errorf("产业链 JSON 解析失败: %w", err)
	}
	// 结构校验
	if len(res.Nodes) == 0 {
		return nil, fmt.Errorf("产业链 nodes 为空")
	}
	if res.Industry == "" {
		res.Industry = "产业链"
	}
	ids := map[string]bool{}
	stageOK := map[string]bool{"上游": true, "中游": true, "下游": true}
	idOK := func(id string) bool {
		if id == "" {
			return false
		}
		for _, r := range id {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
				return false
			}
		}
		return true
	}
	nodes := res.Nodes[:0]
	for _, n := range res.Nodes {
		n.ID = strings.TrimSpace(n.ID)
		n.Stage = strings.TrimSpace(n.Stage)
		if !idOK(n.ID) || !stageOK[n.Stage] {
			continue
		}
		if ids[n.ID] {
			continue
		}
		ids[n.ID] = true
		nodes = append(nodes, n)
	}
	res.Nodes = nodes
	if len(res.Nodes) == 0 {
		return nil, fmt.Errorf("产业链 nodes 全部无效") // 触发外层重试
	}
	// edges 端点必须存在且非自环
	edges := res.Edges[:0]
	for _, e := range res.Edges {
		if ids[e.From] && ids[e.To] && e.From != e.To {
			edges = append(edges, e)
		}
	}
	res.Edges = edges
	// companies 键对应 nodeID
	if res.Companies == nil {
		res.Companies = map[string][]types.ChainCompany{}
	}
	for k, cs := range res.Companies {
		if !ids[k] {
			delete(res.Companies, k)
			continue
		}
		filtered := cs[:0]
		for _, c := range cs {
			if c.Code != "" && c.Name != "" {
				filtered = append(filtered, c)
			}
		}
		if len(filtered) == 0 {
			delete(res.Companies, k)
		} else {
			res.Companies[k] = filtered
		}
	}
	return &res, nil
}
