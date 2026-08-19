// Package finance dao 层：A股财务指标数据（东财双源互备，实测 2026-08-17）
// 主：datacenter API（RPT_F10_FINANCE_MAINFINADATA，支持年报过滤、结构化、稳定）
// 备：F10 页面接口（ZYZBAjaxNew，141 字段）
// 腾讯财务接口实测不可用（F10Controller 不存在），故不设腾讯兜底。
package finance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"zhihudp/internal/types"
)

const browserUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"

// 接口地址（var 便于测试注入 mock）
var (
	datacenterURL = "https://datacenter.eastmoney.com/securities/api/data/v1/get"                    // 主：结构化财务数据
	f10URL        = "https://emweb.securities.eastmoney.com/PC_HSF10/NewFinanceAnalysis/ZYZBAjaxNew" // 备：F10 页面接口
)

// GetResult 获取股票财务指标（5 年年报 + 最新报告期）；主接口失败自动切兜底。
func GetResult(ctx context.Context, code, market string) (*types.FinanceResult, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, fmt.Errorf("股票代码不能为空")
	}
	prefix := marketPrefix(market, code)
	secuCode := code + "." + prefix // datacenter 用：600519.SH
	f10Code := prefix + code        // F10 用：SH600519

	res := &types.FinanceResult{Code: code}

	// 主接口：datacenter（串行，避免与东财其他域并发触发风控）
	items, name, err := fetchDatacenter(ctx, secuCode)
	if err == nil && len(items) > 0 {
		res.Name = name
		res.Indicators = items
		return res, nil
	}
	// 主失败 → 兜底 F10
	items, name, backErr := fetchF10(ctx, f10Code)
	if backErr == nil && len(items) > 0 {
		res.Name = name
		res.Indicators = items
		res.Degraded = true
		res.ErrMsg = "主接口降级: " + errString(err)
		return res, nil
	}
	msg := fmt.Sprintf("主:%v 备:%v", err, backErr)
	res.ErrMsg = msg
	res.Degraded = true
	return res, fmt.Errorf("财务数据获取失败（%s）", msg)
}

// errString 错误转字符串（nil → "无错误"）
func errString(err error) string {
	if err == nil {
		return "无错误（主接口返回空数据）"
	}
	return err.Error()
}

// marketPrefix 市场（沪A/深A/北交）→ 交易所前缀；缺失按代码首位推断
func marketPrefix(market, code string) string {
	switch {
	case strings.Contains(market, "沪"):
		return "SH"
	case strings.Contains(market, "深"):
		return "SZ"
	case strings.Contains(market, "北"):
		return "BJ"
	}
	if code != "" {
		switch code[0] {
		case '6', '9':
			return "SH"
		case '0', '3':
			return "SZ"
		case '4', '8':
			return "BJ"
		}
	}
	return "SH"
}

// fetchDatacenter 主接口：年报过滤取最近 6 期（5 年报 + 自动含最新）
func fetchDatacenter(ctx context.Context, fullCode string) ([]types.FinancialIndicator, string, error) {
	// 1) 5 年年报
	annual, name, err := datacenterQuery(ctx, fullCode, "年报", 6)
	if err != nil {
		return nil, "", err
	}
	// 2) 最新报告期（中报/三季报等）
	latest, _, err := datacenterQuery(ctx, fullCode, "", 1)
	if err == nil && len(latest) > 0 {
		// 若最新报告期与最近年报重复则忽略
		if len(annual) == 0 || latest[0].ReportDateFull != annual[0].ReportDateFull {
			annual = append([]types.FinancialIndicator{latest[0]}, annual...)
		}
	}
	if len(annual) > 6 {
		annual = annual[:6]
	}
	return annual, name, nil
}

// datacenterQuery datacenter API 查询；返回 (指标, 股票名, error)
func datacenterQuery(ctx context.Context, fullCode, reportType string, limit int) ([]types.FinancialIndicator, string, error) {
	q := url.Values{}
	q.Set("reportName", "RPT_F10_FINANCE_MAINFINADATA")
	q.Set("columns", "SECUCODE,SECURITY_CODE,SECURITY_NAME_ABBR,REPORT_DATE,REPORT_DATE_NAME,EPSJB,ROEJQ,XSMLL,XSJLL,ZCFZL,TOTALOPERATEREVE,TOTALOPERATEREVETZ,PARENTNETPROFIT,PARENTNETPROFITTZ,KCFJCXSYJLR,JYXJLYYSR")
	filter := fmt.Sprintf(`(SECUCODE="%s")`, fullCode)
	if reportType != "" {
		filter += fmt.Sprintf(`(REPORT_TYPE="%s")`, reportType)
	}
	q.Set("filter", filter)
	q.Set("pageNumber", "1")
	q.Set("pageSize", strconv.Itoa(limit))
	q.Set("sortTypes", "-1")
	q.Set("sortColumns", "REPORT_DATE")

	body, err := doGet(ctx, datacenterURL+"?"+q.Encode())
	if err != nil {
		return nil, "", err
	}
	var resp struct {
		Result struct {
			Data []struct {
				SecurityNameAbbr string  `json:"SECURITY_NAME_ABBR"`
				ReportDate       string  `json:"REPORT_DATE"`
				ReportDateName   string  `json:"REPORT_DATE_NAME"`
				EPSJB            float64 `json:"EPSJB"`
				ROEJQ            float64 `json:"ROEJQ"`
				XSML             float64 `json:"XSMLL"`
				XSJL             float64 `json:"XSJLL"`
				ZCFZL            float64 `json:"ZCFZL"`
				TotalOperateReve float64 `json:"TOTALOPERATEREVE"`
				RevYoY           float64 `json:"TOTALOPERATEREVETZ"`
				ParentNetProfit  float64 `json:"PARENTNETPROFIT"`
				ProfitYoY        float64 `json:"PARENTNETPROFITTZ"`
				DeductedProfit   float64 `json:"KCFJCXSYJLR"`
				CashFlowToRev    float64 `json:"JYXJLYYSR"`
			} `json:"data"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, "", fmt.Errorf("datacenter 解析失败: %w", err)
	}
	items := make([]types.FinancialIndicator, 0, len(resp.Result.Data))
	name := ""
	for _, d := range resp.Result.Data {
		if name == "" {
			name = d.SecurityNameAbbr
		}
		items = append(items, types.FinancialIndicator{
			ReportDate:     strings.TrimSpace(d.ReportDateName),
			ReportDateFull: shortDate(d.ReportDate),
			Revenue:        yi(d.TotalOperateReve),
			RevenueYoY:     round2(d.RevYoY),
			NetProfit:      yi(d.ParentNetProfit),
			NetProfitYoY:   round2(d.ProfitYoY),
			EPS:            round2(d.EPSJB),
			ROE:            round2(d.ROEJQ),
			GrossMargin:    round2(d.XSML),
			NetMargin:      round2(d.XSJL),
			DebtRatio:      round2(d.ZCFZL),
			CashFlowToRev:  round2(d.CashFlowToRev),
			DeductedProfit: yi(d.DeductedProfit),
		})
	}
	return items, name, nil
}

// fetchF10 兜底接口：F10 页面接口，过滤年报 + 最新报告期
func fetchF10(ctx context.Context, fullCode string) ([]types.FinancialIndicator, string, error) {
	q := url.Values{}
	q.Set("type", "0")
	q.Set("code", fullCode)
	body, err := doGet(ctx, f10URL+"?"+q.Encode())
	if err != nil {
		return nil, "", err
	}
	var resp struct {
		Data []struct {
			SecurityNameAbbr string  `json:"SECURITY_NAME_ABBR"`
			ReportDate       string  `json:"REPORT_DATE"`
			ReportDateName   string  `json:"REPORT_DATE_NAME"`
			EPSJB            float64 `json:"EPSJB"`
			ROEJQ            float64 `json:"ROEJQ"`
			XSML             float64 `json:"XSMLL"`
			XSJL             float64 `json:"XSJLL"`
			ZCFZL            float64 `json:"ZCFZL"`
			TotalOperateReve float64 `json:"TOTALOPERATEREVE"`
			RevYoY           float64 `json:"TOTALOPERATEREVETZ"`
			ParentNetProfit  float64 `json:"PARENTNETPROFIT"`
			ProfitYoY        float64 `json:"PARENTNETPROFITTZ"`
			DeductedProfit   float64 `json:"KCFJCXSYJLR"`
			CashFlowToRev    float64 `json:"JYXJLYYSR"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, "", fmt.Errorf("F10 解析失败: %w", err)
	}
	name := ""
	items := make([]types.FinancialIndicator, 0, len(resp.Data))
	for _, d := range resp.Data {
		if name == "" {
			name = d.SecurityNameAbbr
		}
		if !strings.Contains(d.ReportDate, "12-31") {
			continue // 只要年报（5 年趋势）
		}
		items = append(items, types.FinancialIndicator{
			ReportDate:     strings.TrimSpace(d.ReportDateName),
			ReportDateFull: shortDate(d.ReportDate),
			Revenue:        yi(d.TotalOperateReve),
			RevenueYoY:     round2(d.RevYoY),
			NetProfit:      yi(d.ParentNetProfit),
			NetProfitYoY:   round2(d.ProfitYoY),
			EPS:            round2(d.EPSJB),
			ROE:            round2(d.ROEJQ),
			GrossMargin:    round2(d.XSML),
			NetMargin:      round2(d.XSJL),
			DebtRatio:      round2(d.ZCFZL),
			CashFlowToRev:  round2(d.CashFlowToRev),
			DeductedProfit: yi(d.DeductedProfit),
		})
	}
	if len(items) > 5 {
		items = items[:5]
	}
	return items, name, nil
}

// ---- 工具 ----

// yi 元 → 亿元（保留 2 位小数）
func yi(v float64) float64 {
	if v == 0 {
		return 0
	}
	return round2(v / 1e8)
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}

// shortDate "2025-12-31 00:00:00" → "2025-12-31"
func shortDate(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

func doGet(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Referer", "https://emweb.securities.eastmoney.com/")
	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return body, nil
}
