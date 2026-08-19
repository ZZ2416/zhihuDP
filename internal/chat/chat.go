// Package chat service 层：追问对话的会话存储与上下文事实组装
// 每个股票独立会话；一期 /api/ask 结果快照（分析文本）绑定；
// 对话时实时取行情快照（报价级）与财务/估值数据，注入 agent.Chat。
package chat

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"zhihudp/internal/types"
)

// Snapshot 一期 /api/ask 结果快照（由 handler_ask 捕获事件写入）
type Snapshot struct {
	Stock    types.StockInfo // 股票信息（code/name/market）
	Analysis string          // 一期 AI 分析最终文本
}

// Session 单个股票的对话会话
type Session struct {
	Stock    types.StockInfo
	Messages []types.ChatMessage // 完整历史（组装时按窗口截取）
}

// Store 会话存储（纯内存，重启即失；demo 可接受）
type Store struct {
	mu        sync.Mutex
	sessions  map[string]*Session
	snapshots map[string]Snapshot
	historyN  int // 历史窗口条数
}

// NewStore 创建会话存储
func NewStore(historyN int) *Store {
	if historyN < 2 {
		historyN = 10
	}
	return &Store{
		sessions:  map[string]*Session{},
		snapshots: map[string]Snapshot{},
		historyN:  historyN,
	}
}

// SetSnapshot 保存/覆盖一期分析快照（handler_ask 结束后调用）
func (s *Store) SetSnapshot(code string, snap Snapshot) {
	if code == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshots[code] = snap
}

// SnapshotOf 读取快照
func (s *Store) SnapshotOf(code string) (Snapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, ok := s.snapshots[code]
	return snap, ok
}

// GetOrCreate 获取会话；首次创建时绑定快照中的股票信息
func (s *Store) GetOrCreate(code string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getOrCreateLocked(code)
}

// getOrCreateLocked 已持锁的 GetOrCreate
func (s *Store) getOrCreateLocked(code string) *Session {
	if sess, ok := s.sessions[code]; ok {
		return sess
	}
	sess := &Session{Stock: types.StockInfo{Code: code}}
	if snap, ok := s.snapshots[code]; ok {
		sess.Stock = snap.Stock
	}
	s.sessions[code] = sess
	return sess
}

// Append 追加一条消息（user / assistant）；会话不存在则自动创建
func (s *Store) Append(code, role, content string) {
	if strings.TrimSpace(content) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.getOrCreateLocked(code)
	sess.Messages = append(sess.Messages, types.ChatMessage{Role: role, Content: content})
	// 窗口裁剪：保留最近 historyN 条
	if len(sess.Messages) > s.historyN {
		sess.Messages = sess.Messages[len(sess.Messages)-s.historyN:]
	}
}

// Reset 清空会话（前端「清空」按钮）
func (s *Store) Reset(code string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, code)
}

// History 返回窗口内历史消息
func (s *Store) History(code string) []types.ChatMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[code]; ok {
		return append([]types.ChatMessage(nil), sess.Messages...)
	}
	return nil
}

// Deps chat 服务依赖（函数注入，便于测试与替换）
type Deps struct {
	// Quote 实时取行情快照（报价级）：由 kline.GetKline(market, code, 1) 包装
	Quote func(ctx context.Context, market, code string) (*types.Quote, error)
	// Finance 财报指标（5年年报+最新期）；由 finance.GetResult 包装，可为 nil（跳过）
	Finance func(ctx context.Context, code, market string) (*types.FinanceResult, error)
	// ChatAgent 调用「AI 看山」对话（internal/agent.Chat 包装），SSE 事件经 sink 转发
	ChatAgent func(ctx context.Context, facts types.ChatFacts, history []types.ChatMessage, message string, sink func(types.Event) error) error
}

// Service 对话编排：会话管理 + 上下文组装 + 调用 agent
type Service struct {
	store *Store
	deps  Deps
}

// New 创建对话服务
func New(store *Store, deps Deps) *Service { return &Service{store: store, deps: deps} }

// SetSnapshot 实现 server.ChatProvider：一期 /api/ask 结束后写入结果快照
func (s *Service) SetSnapshot(code string, stock types.StockInfo, analysis string) {
	s.store.SetSnapshot(code, Snapshot{Stock: stock, Analysis: analysis})
}

// Reset 实现 server.ChatProvider：清空某股票会话
func (s *Service) Reset(code string) { s.store.Reset(code) }

// Chat 处理一次追问（SSE 事件经 sink 转发），完成后把看山回复写入会话
func (s *Service) Chat(ctx context.Context, code, market, message string, sink func(types.Event) error) error {
	sess := s.store.GetOrCreate(code)
	// 1) 记录用户消息
	s.store.Append(code, "user", message)
	// 2) 组装上下文事实
	facts, err := s.buildFacts(ctx, sess, market)
	if err != nil {
		return fmt.Errorf("组装对话上下文失败: %w", err)
	}
	// 3) 调用 AI 看山（内部把 delta 累积并转发给 sink）
	history := s.store.History(code)
	// 排除刚追加的这条 user 消息外的历史（agent 端会带上本条 message）
	var reply strings.Builder
	inner := func(ev types.Event) error {
		if ev.Type == "delta" {
			if m, ok := ev.Data.(map[string]string); ok {
				reply.WriteString(m["text"])
			}
		}
		return sink(ev)
	}
	if err := s.deps.ChatAgent(ctx, facts, history[:len(history)-1], message, inner); err != nil {
		return err
	}
	// 4) 记录看山回复
	s.store.Append(code, "assistant", reply.String())
	return nil
}

// buildFacts 组装对话上下文：股票 → 行情 → 情绪 → 知识库 → 前次分析
func (s *Service) buildFacts(ctx context.Context, sess *Session, market string) (types.ChatFacts, error) {
	facts := types.ChatFacts{
		StockName: sess.Stock.Name,
		StockCode: sess.Stock.Code,
		Market:    firstNonEmpty(market, sess.Stock.Market),
	}
	// 行情快照（报价级，实时）
	if s.deps.Quote != nil && facts.StockCode != "" {
		if q, err := s.deps.Quote(ctx, facts.Market, facts.StockCode); err == nil && q != nil {
			facts.Quote = fmt.Sprintf(
				"现价 %.2f，涨跌 %+.2f（%+.2f%%），今开 %.2f，最高 %.2f，最低 %.2f，成交量 %.0f 手",
				q.Price, q.Change, q.ChangePct, q.Open, q.High, q.Low, q.Volume)
		}
	}
	// 前次分析快照
	if snap, ok := s.store.SnapshotOf(sess.Stock.Code); ok {
		facts.PrevAnalysis = snap.Analysis
		if sess.Stock.Name == "" {
			sess.Stock = snap.Stock
		}
	}
	// 财报指标摘要（5年年报+最新期，最小事实注入）
	if s.deps.Finance != nil && facts.StockCode != "" {
		if res, err := s.deps.Finance(ctx, facts.StockCode, facts.Market); err == nil && res != nil && len(res.Indicators) > 0 {
			facts.Finance = formatFinance(res.Indicators)
		}
	}
	return facts, nil
}

// formatFinance 财务指标 → 紧凑文本（报告期 | 营收 | 净利 | ROE | 毛利率 | 负债率）
func formatFinance(items []types.FinancialIndicator) string {
	var b strings.Builder
	b.WriteString("最近5年年报+最新期（单位：亿元/%）：\n")
	for _, it := range items {
		b.WriteString(fmt.Sprintf("%s：营收%.1f(同比%+.1f%%) 净利%.1f(同比%+.1f%%) EPS%.2f ROE%.1f%% 毛利率%.1f%% 净利率%.1f%% 负债率%.1f%%\n",
			it.ReportDate, it.Revenue, it.RevenueYoY, it.NetProfit, it.NetProfitYoY,
			it.EPS, it.ROE, it.GrossMargin, it.NetMargin, it.DebtRatio))
	}
	return b.String()
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
