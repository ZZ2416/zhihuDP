// Package valuation dao 层：估值数据（统一东财 PE_TTM 口径 + 腾讯兜底，实测 2026-08-19）
// 当前 PE 与历史分位同源（东财 RPT_VALUEANALYSIS_DET，避免口径不一致）；
// 腾讯 qt.gtimg 提供 PB/市值并作为 PE 兜底（分位此时 -1 标注）。
package valuation

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

var (
	emValueURL = "https://datacenter.eastmoney.com/securities/api/data/v1/get" // 主：历史 PE_TTM 序列
	txQtURL    = "https://qt.gtimg.cn/q="                                      // 备：当前 PE/PB/市值
)

// percentileDays 分位序列天数（东财实测约 419 个交易日）
const percentileDays = 419

// Get 获取估值：东财 PE_TTM 序列（当前 PE + 分位）主，腾讯 qt 补充 PB/市值并兜底
// 参数顺序与 finance.GetResult 一致：(ctx, code, market)
func Get(ctx context.Context, code, market string) (*types.Valuation, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, fmt.Errorf("股票代码不能为空")
	}
	// PEEntPercent 初始 -1（无分位）；分位合法范围 0-100（0 = 当前为历史最低）
	v := &types.Valuation{PEEntPercent: -1}

	// 主：东财 PE_TTM 历史序列（含负值）→ 当前 PE + 分位
	peSeries, err := fetchEMSeries(ctx, market, code)
	if err == nil && len(peSeries) > 0 {
		cur := peSeries[len(peSeries)-1] // 最新一个交易日
		v.PE = cur
		if cur > 0 {
			// 分位仅基于正 PE 序列（亏损/负 PE 无意义）
			v.PEEntPercent = percentile(positiveOnly(peSeries), cur)
		}
	} else {
		v.Degraded = true
	}

	// 腾讯 qt：PB / 市值，PE 兜底（东财失败或 PE 缺失时）
	if pb, mcap, pe, txErr := fetchTx(ctx, market, code); txErr == nil {
		v.PB = pb
		v.MarketCap = mcap
		if v.PE <= 0 && pe > 0 {
			v.PE = pe
			// 腾讯兜底无历史序列 → 分位保持 -1
		}
	} else if err != nil {
		return nil, fmt.Errorf("估值获取失败（东财:%v 腾讯:%v）", err, txErr)
	}

	// PE≤0（亏损）估值无意义：分位 -1
	if v.PE <= 0 {
		v.PEEntPercent = -1
	}
	return v, nil
}

// positiveOnly 过滤正 PE 序列（分位计算用）
func positiveOnly(series []float64) []float64 {
	out := make([]float64, 0, len(series))
	for _, v := range series {
		if v > 0 {
			out = append(out, v)
		}
	}
	return out
}

// fetchEMSeries 东财每日 PE_TTM 序列（升序日期，取最近 percentileDays 条）
func fetchEMSeries(ctx context.Context, market, code string) ([]float64, error) {
	secu := code + "." + secidSuffix(market, code)
	q := url.Values{}
	q.Set("reportName", "RPT_VALUEANALYSIS_DET")
	q.Set("columns", "PE_TTM,TRADE_DATE")
	q.Set("filter", fmt.Sprintf(`(SECUCODE="%s")`, secu))
	q.Set("pageNumber", "1")
	q.Set("pageSize", strconv.Itoa(percentileDays))
	q.Set("sortTypes", "-1")
	q.Set("sortColumns", "TRADE_DATE")

	body, err := doGet(ctx, emValueURL+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	var resp struct {
		Result struct {
			Data []struct {
				PE_TTM float64 `json:"PE_TTM"`
			} `json:"data"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("东财估值解析失败: %w", err)
	}
	// 返回降序（最新在前），翻转为升序（旧→新）；保留负 PE（当前 PE 取真实最新值，分位另过滤）
	series := make([]float64, 0, len(resp.Result.Data))
	for _, d := range resp.Result.Data {
		series = append(series, d.PE_TTM)
	}
	for i, j := 0, len(series)-1; i < j; i, j = i+1, j-1 {
		series[i], series[j] = series[j], series[i]
	}
	if len(series) == 0 {
		return nil, fmt.Errorf("东财估值序列为空")
	}
	return series, nil
}

// fetchTx 腾讯 qt.gtimg.cn：PB/市值/PE（GBK 字符串）
func fetchTx(ctx context.Context, market, code string) (pb, mcap, pe float64, err error) {
	prefix := "sh"
	if strings.Contains(market, "深") || (code != "" && (code[0] == '0' || code[0] == '3')) {
		prefix = "sz"
	}
	if strings.Contains(market, "北") || (code != "" && (code[0] == '4' || code[0] == '8')) {
		prefix = "bj"
	}
	body, err := doGet(ctx, txQtURL+prefix+code)
	if err != nil {
		return 0, 0, 0, err
	}
	// 仅取数字字段（ASCII，不受 GBK 名称影响）
	fields := strings.Split(string(body), "~")
	// 已知字段：PE[39]、PB[46]、总市值[45]（亿）
	getF := func(i int) float64 {
		if i >= len(fields) {
			return 0
		}
		v, _ := strconv.ParseFloat(fields[i], 64)
		return v
	}
	return getF(46), getF(45), getF(39), nil
}

// percentile 当前值在序列中的百分位（0-100）
func percentile(sorted []float64, cur float64) float64 {
	n := len(sorted)
	if n == 0 {
		return -1
	}
	cnt := 0
	for _, v := range sorted {
		if v <= cur {
			cnt++
		}
	}
	return float64(cnt) / float64(n) * 100
}

// secidSuffix 东财 SECUCODE 后缀（沪=SH，深/北=SZ）
func secidSuffix(market, code string) string {
	if strings.Contains(market, "沪") || (code != "" && (code[0] == '6' || code[0] == '9')) {
		return "SH"
	}
	return "SZ"
}

func doGet(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", browserUA)
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
