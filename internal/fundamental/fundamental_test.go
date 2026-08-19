package fundamental

import (
	"context"
	"testing"

	"zhihudp/internal/types"
)

// 高质地样本：ROE 30 / 毛利 80 / 净利 40，高成长，低负债，低估
func goodIndicators() []types.FinancialIndicator {
	return []types.FinancialIndicator{
		{ReportDate: "2026中报", ReportDateFull: "2026-06-30", Revenue: 200, RevenueYoY: 20, NetProfit: 60, NetProfitYoY: 25, EPS: 5, ROE: 18, GrossMargin: 80, NetMargin: 30, DebtRatio: 25, CashFlowToRev: 0.4},
		{ReportDate: "2025年报", ReportDateFull: "2025-12-31", Revenue: 180, ROE: 20, GrossMargin: 82, NetMargin: 28, DebtRatio: 26, CashFlowToRev: 0.4},
		{ReportDate: "2021年报", ReportDateFull: "2021-12-31", Revenue: 120, ROE: 15, GrossMargin: 80, NetMargin: 25, DebtRatio: 30, CashFlowToRev: 0.3},
	}
}

// 差质地样本：低 ROE 负成长高负债
func poorIndicators() []types.FinancialIndicator {
	return []types.FinancialIndicator{
		{ReportDate: "2026中报", ReportDateFull: "2026-06-30", Revenue: 50, RevenueYoY: -15, NetProfit: 1, NetProfitYoY: -30, EPS: 0.1, ROE: 2, GrossMargin: 8, NetMargin: 2, DebtRatio: 85, CashFlowToRev: -0.2},
		{ReportDate: "2021年报", ReportDateFull: "2021-12-31", Revenue: 80, ROE: 5, GrossMargin: 10, NetMargin: 3, DebtRatio: 70, CashFlowToRev: 0.1},
	}
}

func TestScoreGoodCompany(t *testing.T) {
	s := compute(goodIndicators(), &types.Valuation{PE: 10, PEEntPercent: 10})
	if s.Profit < 70 || s.Growth < 70 || s.Health < 60 || s.Valuat < 90 {
		t.Errorf("优质公司各维应高分: %+v", s)
	}
	if s.Total < 75 {
		t.Errorf("优质公司总分应≥75: %d %s", s.Total, s.Grade)
	}
	if s.Grade != "质地强" {
		t.Errorf("grade=%s want 质地强", s.Grade)
	}
	if len(s.NoData) != 0 {
		t.Errorf("不应有数据不足: %v", s.NoData)
	}
}

func TestScorePoorCompany(t *testing.T) {
	s := compute(poorIndicators(), &types.Valuation{PE: -5, PEEntPercent: -1})
	if s.Total >= 40 {
		t.Errorf("差质地公司总分应<40: %d", s.Total)
	}
	if s.Grade != "质地偏弱" {
		t.Errorf("grade=%s want 质地偏弱", s.Grade)
	}
}

func TestScoreDataMissing(t *testing.T) {
	// 只有最新期营收（无 5 年前 → 成长降权、无估值分位 → 估值降权）
	its := []types.FinancialIndicator{
		{ReportDate: "2026中报", ReportDateFull: "2026-06-30", Revenue: 100, ROE: 20, GrossMargin: 50, NetMargin: 20, DebtRatio: 30, CashFlowToRev: 0.3},
	}
	s := compute(its, &types.Valuation{PE: 0, PEEntPercent: -1})
	if len(s.NoData) == 0 {
		t.Errorf("应有数据不足标注: %v", s.NoData)
	}
	if s.Total <= 0 || s.Total > 100 {
		t.Errorf("总分范围异常: %d", s.Total)
	}
}

func TestLin(t *testing.T) {
	xs := []float64{0, 10, 20}
	ys := []float64{0, 50, 100}
	if got := lin(5, xs, ys); got != 25 {
		t.Errorf("lin(5)=%v want 25", got)
	}
	if got := lin(0, xs, ys); got != 0 {
		t.Errorf("lin(0)=%v want 0", got)
	}
	if got := lin(30, xs, ys); got != 100 {
		t.Errorf("lin(30)=%v want 100", got)
	}
}

// Score 端到端（注入 mock finance + valuation）
func TestScoreE2E(t *testing.T) {
	svc := New(Deps{
		Finance: func(_ context.Context, code, _ string) (*types.FinanceResult, error) {
			return &types.FinanceResult{Code: code, Name: "贵州茅台", Indicators: goodIndicators()}, nil
		},
		Valuation: func(_ context.Context, _, _ string) (*types.Valuation, error) {
			return &types.Valuation{PE: 15, PEEntPercent: 20, PB: 5, MarketCap: 16000}, nil
		},
	})
	res, err := svc.Score(context.Background(), "600519", "沪A")
	if err != nil {
		t.Fatalf("Score 失败: %v", err)
	}
	if res.Name != "贵州茅台" {
		t.Errorf("name=%q", res.Name)
	}
	if res.Valuation.PEEntPercent != 20 {
		t.Errorf("分位=%v want 20", res.Valuation.PEEntPercent)
	}
	if res.Score.Total < 75 {
		t.Errorf("优质 mock 总分应≥75: %d", res.Score.Total)
	}
}

// CAGR 用实际年数（3 年数据 → 开 3 次方，而非固定 5）
func TestCagrDynamicYears(t *testing.T) {
	its := []types.FinancialIndicator{
		{ReportDate: "2026中报", ReportDateFull: "2026-06-30", Revenue: 200, RevenueYoY: 20, NetProfitYoY: 20,
			ROE: 20, GrossMargin: 60, NetMargin: 20, DebtRatio: 30, CashFlowToRev: 0.3},
		{ReportDate: "2023年报", ReportDateFull: "2023-12-31", Revenue: 100, ROE: 20, GrossMargin: 60, NetMargin: 20, DebtRatio: 30, CashFlowToRev: 0.3},
	}
	// 200/100 开 3 次方 - 1 = 26%，应得到成长高分（≥70）
	s := compute(its, &types.Valuation{PE: 15, PEEntPercent: 30})
	if s.Growth < 70 {
		t.Errorf("3年翻倍 CAGR=26%% 应高成长分，实际 %d", s.Growth)
	}
}

// ROE 趋势同口径：中报 ROE 不参与对比（最近年报 vs 最早年报）
func TestRoeTrendAnnualOnly(t *testing.T) {
	// 中报 ROE 低（16.75）但年报 ROE 稳定（32/33）→ 趋势不应被中报拉低
	its := []types.FinancialIndicator{
		{ReportDate: "2026中报", ReportDateFull: "2026-06-30", ROE: 16.75},
		{ReportDate: "2025年报", ReportDateFull: "2025-12-31", ROE: 32.5},
		{ReportDate: "2021年报", ReportDateFull: "2021-12-31", ROE: 33.0},
	}
	if got := roeTrend(its); got != 0 {
		t.Errorf("年报 ROE 32.5 vs 33 趋势应为 0，实际 %v", got)
	}
	// 年报 ROE 明显下滑 → -10
	its2 := []types.FinancialIndicator{
		{ReportDate: "2025年报", ReportDateFull: "2025-12-31", ROE: 20},
		{ReportDate: "2021年报", ReportDateFull: "2021-12-31", ROE: 35},
	}
	if got := roeTrend(its2); got != -10 {
		t.Errorf("ROE 35→20 趋势应 -10，实际 %v", got)
	}
}
