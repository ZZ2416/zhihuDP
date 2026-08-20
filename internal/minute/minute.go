// Package minute dao 层：当日分时数据（东财主 + 腾讯兜底，实测 2026-08-19）
package minute

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
	emTrendsURL = "https://push2his.eastmoney.com/api/qt/stock/trends2/get" // 主：东财分时
	txMinuteURL = "https://web.ifzq.gtimg.cn/appstock/app/minute/query"     // 备：腾讯分时
)

// GetMinute 获取当日分时；主（东财）失败自动切兜底（腾讯）
func GetMinute(ctx context.Context, market, code string) (*types.MinuteResult, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, fmt.Errorf("股票代码不能为空")
	}
	res, err := fetchEM(ctx, market, code)
	if err == nil && len(res.Points) > 0 {
		return res, nil
	}
	back, backErr := fetchTx(ctx, market, code)
	if backErr == nil && len(back.Points) > 0 {
		back.Degraded = true
		return back, nil
	}
	return nil, fmt.Errorf("分时数据获取失败（主:%v 备:%v）", err, backErr)
}

// secid 东财证券标识：市场.代码（沪=1，深/北=0）
func secid(market, code string) string {
	m := "0"
	if strings.Contains(market, "沪") || (code != "" && (code[0] == '6' || code[0] == '9')) {
		m = "1"
	}
	return m + "." + code
}

// fetchEM 东财分时（trends2，含均价线与昨收）
func fetchEM(ctx context.Context, market, code string) (*types.MinuteResult, error) {
	q := url.Values{}
	q.Set("secid", secid(market, code))
	q.Set("fields1", "f1,f2,f3,f4,f5,f6,f7,f8,f9,f10,f11,f12,f13")
	q.Set("fields2", "f51,f52,f53,f54,f55,f56,f57,f58")
	q.Set("ndays", "1")
	q.Set("iscr", "0")

	body, err := doGet(ctx, emTrendsURL+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data struct {
			Code      string   `json:"code"`
			Name      string   `json:"name"`
			PreClose  float64  `json:"preClose"`
			Trends    []string `json:"trends"`
			TrendsTot int      `json:"trendsTotal"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("东财分时解析失败: %w", err)
	}
	res := &types.MinuteResult{Code: code, Name: resp.Data.Name, PreClose: resp.Data.PreClose}
	for _, line := range resp.Data.Trends {
		// 格式: 时间,开,收,高,低,量,额,均价
		f := strings.Split(line, ",")
		if len(f) < 8 {
			continue
		}
		p, _ := strconv.ParseFloat(f[1], 64)
		avg, _ := strconv.ParseFloat(f[7], 64)
		vol, _ := strconv.ParseFloat(f[5], 64)
		res.Points = append(res.Points, types.MinutePoint{
			Time:     strings.TrimSpace(strings.Split(f[0], " ")[1]),
			Price:    p,
			AvgPrice: avg,
			Volume:   vol,
		})
	}
	return res, nil
}

// fetchTx 腾讯分时（无均价线，兜底）
func fetchTx(ctx context.Context, market, code string) (*types.MinuteResult, error) {
	prefix := "sh"
	if strings.Contains(market, "深") || (code != "" && (code[0] == '0' || code[0] == '3')) {
		prefix = "sz"
	}
	if strings.Contains(market, "北") || (code != "" && (code[0] == '4' || code[0] == '8')) {
		prefix = "bj"
	}
	q := url.Values{}
	q.Set("code", prefix+code)
	body, err := doGet(ctx, txMinuteURL+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	var resp struct {
		Code int `json:"code"`
		Data map[string]struct {
			Data struct {
				Data []string `json:"data"`
				Date string   `json:"date"`
				Qt   struct {
					PreClose float64 `json:"prec"`
				} `json:"qt"`
			} `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.Code != 0 {
		return nil, fmt.Errorf("腾讯分时解析失败: %v", err)
	}
	key := prefix + code
	inner, ok := resp.Data[key]
	if !ok {
		return nil, fmt.Errorf("腾讯分时无 %s 数据", key)
	}
	res := &types.MinuteResult{Code: code, PreClose: inner.Data.Qt.PreClose}
	var cumVol, cumAmt float64
	for _, line := range inner.Data.Data {
		// 格式: HHMM 价格 成交量 成交额
		f := strings.Split(line, " ")
		if len(f) < 4 {
			continue
		}
		p, _ := strconv.ParseFloat(f[1], 64)
		vol, _ := strconv.ParseFloat(f[2], 64)
		amt, _ := strconv.ParseFloat(f[3], 64)
		cumVol += vol
		cumAmt += amt
		avg := 0.0
		if cumVol > 0 {
			avg = cumAmt / cumVol / 100 // 腾讯量为手、额为元 → 均价=额/量/100
		}
		res.Points = append(res.Points, types.MinutePoint{
			Time:     f[0][:2] + ":" + f[0][2:],
			Price:    p,
			AvgPrice: avg,
			Volume:   vol,
		})
	}
	return res, nil
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
