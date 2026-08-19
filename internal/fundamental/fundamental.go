// Package fundamental service 层：四维基本面评分模型（盈利/成长/财务健康/估值）
// 输入：财务指标 + 估值 → 输出 FundamentalScore。评分纯函数，依赖注入便于测试。
package fundamental

import (
	"context"
	"fmt"
	"math"

	"zhihudp/internal/types"
)

// 维度权重（数据不足时剔除并按剩余归一）
const (
	wProfit = 0.30
	wGrowth = 0.25
	wHealth = 0.25
	wValuat = 0.20
)

// Deps 依赖注入（测试可替换）
type Deps struct {
	Finance   func(ctx context.Context, code, market string) (*types.FinanceResult, error)
	Valuation func(ctx context.Context, market, code string) (*types.Valuation, error)
}

// Service 基本面评分服务
type Service struct {
	deps Deps
}

// New 创建评分服务
func New(deps Deps) *Service { return &Service{deps: deps} }

// Score 聚合财务 + 估值 → 四维评分
func (s *Service) Score(ctx context.Context, code, market string) (*types.FundamentalResult, error) {
	fres, err := s.deps.Finance(ctx, code, market)
	if err != nil {
		return nil, fmt.Errorf("财务数据: %w", err)
	}
	val, err := s.deps.Valuation(ctx, market, code)
	if err != nil {
		// 估值失败不阻塞评分（估值维 50 + 标注）
		val = &types.Valuation{PE: 0, PEEntPercent: -1, Degraded: true}
	}

	res := &types.FundamentalResult{
		Code:       code,
		Name:       fres.Name,
		Indicators: fres.Indicators,
		Valuation:  *val,
		Degraded:   fres.Degraded || val.Degraded,
	}
	res.Score = compute(fres.Indicators, val)
	return res, nil
}

// compute 四维评分（纯函数）
func compute(indicators []types.FinancialIndicator, v *types.Valuation) types.FundamentalScore {
	// 最新期 + 最早报告期（用于 CAGR 与趋势基准）
	var latest, base types.FinancialIndicator
	if len(indicators) > 0 {
		latest = indicators[0] // 组装时最新在前
	}
	for _, it := range indicators {
		if it.ReportDateFull == "" {
			continue
		}
		if base.ReportDateFull == "" || it.ReportDateFull < base.ReportDateFull {
			base = it
		}
	}

	s := types.FundamentalScore{}
	weights := map[string]float64{"profit": wProfit, "growth": wGrowth, "health": wHealth, "valuat": wValuat}

	// 盈利
	if latest.ROE > 0 || latest.GrossMargin > 0 || latest.NetMargin > 0 {
		roe := lin(latest.ROE, []float64{0, 5, 10, 15, 20, 25}, []float64{10, 30, 50, 70, 85, 100})
		gm := lin(latest.GrossMargin, []float64{10, 15, 25, 40, 60}, []float64{25, 40, 55, 75, 100})
		nm := lin(latest.NetMargin, []float64{0, 5, 10, 20, 30}, []float64{20, 40, 60, 80, 100})
		trend := 0.0
		if base.ROE > 0 && latest.ROE > 0 {
			if latest.ROE-base.ROE >= 5 {
				trend = 10
			} else if base.ROE-latest.ROE >= 5 {
				trend = -10
			}
		}
		s.Profit = int(clamp(roe*0.4 + gm*0.3 + nm*0.3 + trend))
	} else {
		weights["profit"] = 0
		s.NoData = append(s.NoData, "盈利")
	}

	// 成长：5年营收CAGR + 最近净利同比
	if (base.Revenue > 0 && latest.Revenue > 0) || latest.NetProfitYoY != 0 {
		cagr := 0.0
		if base.Revenue > 0 && latest.Revenue > 0 {
			years := 5.0
			cagr = math.Pow(latest.Revenue/base.Revenue, 1/years)*100 - 100
		}
		cg := lin(cagr, []float64{0, 5, 10, 15, 20, 30}, []float64{15, 35, 50, 65, 80, 100})
		ny := lin(latest.NetProfitYoY, []float64{-20, -10, 0, 5, 15, 30}, []float64{10, 30, 50, 65, 80, 100})
		s.Growth = int(clamp(cg*0.5 + ny*0.5))
	} else {
		weights["growth"] = 0
		s.NoData = append(s.NoData, "成长")
	}

	// 财务健康
	if latest.DebtRatio > 0 || latest.CashFlowToRev != 0 {
		dr := lin(latest.DebtRatio, []float64{20, 30, 40, 50, 60, 70, 80}, []float64{100, 85, 70, 55, 40, 25, 10})
		cf := lin(latest.CashFlowToRev, []float64{-0.1, 0, 0.15, 0.3, 0.5}, []float64{10, 40, 60, 80, 100})
		s.Health = int(clamp(dr*0.6 + cf*0.4))
	} else {
		weights["health"] = 0
		s.NoData = append(s.NoData, "财务健康")
	}

	// 估值：PE 历史分位
	if v.PEEntPercent >= 0 {
		p := v.PEEntPercent
		var vs float64
		switch {
		case p <= 20:
			vs = 100
		case p >= 80:
			vs = 10
		default:
			vs = 100 - (p-20)/(60)*70 // 20-80 → 100-30 线性
		}
		s.Valuat = int(clamp(vs))
	} else {
		weights["valuat"] = 0
		s.Valuat = 50
		s.NoData = append(s.NoData, "估值")
	}

	// 加权总分（数据不足维度剔除后归一）
	totalW := 0.0
	for _, w := range weights {
		totalW += w
	}
	if totalW <= 0 {
		s.Total = 0
		s.Grade = "数据不足"
		return s
	}
	sum := float64(s.Profit)*weights["profit"] +
		float64(s.Growth)*weights["growth"] +
		float64(s.Health)*weights["health"] +
		float64(s.Valuat)*weights["valuat"]
	s.Total = int(clamp(sum / totalW))
	s.Grade = gradeOf(s.Total)
	return s
}

// gradeOf 总分定性
func gradeOf(total int) string {
	switch {
	case total >= 75:
		return "质地强"
	case total >= 60:
		return "质地良好"
	case total >= 40:
		return "质地一般"
	default:
		return "质地偏弱"
	}
}

// lin 分段线性插值：xs 升序，ys 对应值
func lin(x float64, xs, ys []float64) float64 {
	if x <= xs[0] {
		return ys[0]
	}
	for i := 1; i < len(xs); i++ {
		if x <= xs[i] {
			t := (x - xs[i-1]) / (xs[i] - xs[i-1])
			return ys[i-1] + (ys[i]-ys[i-1])*t
		}
	}
	return ys[len(ys)-1]
}

func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
