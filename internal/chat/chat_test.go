package chat

import (
	"context"
	"strings"
	"testing"

	"zhihudp/internal/types"
)

func TestSessionIsolation(t *testing.T) {
	store := NewStore(10)
	store.SetSnapshot("600519", Snapshot{Stock: types.StockInfo{Code: "600519", Name: "贵州茅台", Market: "沪A"}})
	store.SetSnapshot("000001", Snapshot{Stock: types.StockInfo{Code: "000001", Name: "平安银行", Market: "深A"}})

	store.Append("600519", "user", "茅台情绪如何？")
	store.Append("600519", "assistant", "看山觉得…")
	store.Append("000001", "user", "平安银行呢？")

	h1 := store.History("600519")
	h2 := store.History("000001")
	if len(h1) != 2 || len(h2) != 1 {
		t.Fatalf("会话串扰：600519 历史 %d 条，000001 历史 %d 条", len(h1), len(h2))
	}
	if store.GetOrCreate("600519").Stock.Name != "贵州茅台" {
		t.Errorf("600519 会话未绑定快照股票信息")
	}
}

func TestHistoryWindow(t *testing.T) {
	store := NewStore(3)
	store.SetSnapshot("600519", Snapshot{Stock: types.StockInfo{Code: "600519"}})
	for i := 0; i < 6; i++ {
		store.Append("600519", "user", "q")
		store.Append("600519", "assistant", "a")
	}
	if got := len(store.History("600519")); got != 3 {
		t.Fatalf("历史窗口应为 3，实际 %d", got)
	}
}

func TestReset(t *testing.T) {
	store := NewStore(10)
	store.SetSnapshot("600519", Snapshot{Stock: types.StockInfo{Code: "600519"}})
	store.Append("600519", "user", "q")
	store.Reset("600519")
	if got := store.History("600519"); len(got) != 0 {
		t.Fatalf("Reset 后历史应为空，实际 %d 条", len(got))
	}
}

// fakeDeps 测试用依赖
func fakeDeps(t *testing.T) Deps {
	return Deps{
		Quote: func(_ context.Context, _, _ string) (*types.Quote, error) {
			return &types.Quote{Price: 100, Change: 1.5, ChangePct: 1.52, Open: 99, High: 101, Low: 98, Volume: 12345}, nil
		},
		Knowledge: func(_ context.Context, _ string, _ int) ([]types.KnowledgeItem, error) {
			return []types.KnowledgeItem{{DocName: "如何做好仓位管理", Content: []string{"这是一段方法论内容。"}}}, nil
		},
		ChatAgent: func(_ context.Context, facts types.ChatFacts, history []types.ChatMessage, message string, sink func(types.Event) error) error {
			// 校验事实组装与历史
			if !strings.Contains(facts.Quote, "现价 100.00") {
				t.Errorf("行情快照缺失: %q", facts.Quote)
			}
			if !strings.Contains(facts.Sentiment, "看多 80%") {
				t.Errorf("情绪摘要缺失: %q", facts.Sentiment)
			}
			if !strings.Contains(facts.PrevAnalysis, "一期分析") {
				t.Errorf("前次分析缺失: %q", facts.PrevAnalysis)
			}
			if len(history) != 2 {
				t.Errorf("历史应含 2 条（不含新提问），实际 %d", len(history))
			}
			return sink(types.Event{Type: "delta", Data: map[string]string{"text": "看山回复"}})
		},
	}
}

func TestChatFlow(t *testing.T) {
	store := NewStore(10)
	svc := New(store, fakeDeps(t))
	store.SetSnapshot("600519", Snapshot{
		Stock:     types.StockInfo{Code: "600519", Name: "贵州茅台", Market: "沪A"},
		Sentiment: &types.SentimentResult{Heat: 10, Sample: 10, Ratio: types.Ratio{Bull: 0.8, Bear: 0.1, Neutral: 0.1}},
		Analysis:  "一期分析：情绪偏多。",
	})
	// 先有一次历史
	store.Append("600519", "user", "之前的问题")
	store.Append("600519", "assistant", "之前的回答")

	var events []string
	sink := func(ev types.Event) error {
		events = append(events, ev.Type)
		return nil
	}
	if err := svc.Chat(context.Background(), "600519", "沪A", "情绪为什么偏空？", sink); err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}
	if len(events) != 1 || events[0] != "delta" {
		t.Fatalf("事件应为 1 个 delta，实际 %v", events)
	}
	// 助手回复已写入会话
	h := store.History("600519")
	if last := h[len(h)-1]; last.Role != "assistant" || !strings.Contains(last.Content, "看山回复") {
		t.Fatalf("助手回复未写入会话: %+v", last)
	}
	if n := len(h); n != 4 { // 历史2 + user + assistant
		t.Fatalf("消息总数应为 4，实际 %d", n)
	}
}
