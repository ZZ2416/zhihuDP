// Package kline dao 层：行情数据访问（腾讯日K线 + 实时报价，前复权）
// 单请求同时返回 K线(qfqday) 与 报价(qt)，实测稳定（2026-08-16，东财同接口曾限流故弃用）
package kline

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

// GetKline 获取股票报价与日K线（腾讯接口，单请求，超时 5s）
// market: 沪A/深A/北A（也接受 sh/sz/bj）；days: K线根数，clamp [10,250]
func GetKline(ctx context.Context, market, code string, days int) (*types.Kline, error) {
	mkt, err := mktCode(market)
	if err != nil {
		return nil, err
	}
	if days < 10 {
		days = 10
	}
	if days > 250 {
		days = 250
	}

	u := "https://web.ifzq.gtimg.cn/appstock/app/fqkline/get"
	q := url.Values{}
	q.Set("param", mkt+code+",day,,,"+strconv.Itoa(days)+",qfq")

	body, err := doGet(ctx, u+"?"+q.Encode())
	if err != nil {
		return nil, fmt.Errorf("腾讯行情请求失败: %w", err)
	}

	var resp struct {
		Code int `json:"code"`
		Data map[string]struct {
			// 除权除息日的数据行末尾会带一个分红对象（如 {"nd":"2025","FHcontent":"10派280.242元"}），
			// 故用 RawMessage 宽容解析，只取前 6 个字符串字段
			QFQDay [][]json.RawMessage   `json:"qfqday"`
			QT     map[string][]string   `json:"qt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("腾讯行情解析失败: %w", err)
	}

	key := mkt + code
	d, ok := resp.Data[key]
	if !ok {
		return nil, fmt.Errorf("腾讯行情无数据: %s", key)
	}

	// K线：[日期,开盘,收盘,最高,最低,成交量(手)]，忽略行尾分红对象
	candles := make([]types.Candle, 0, len(d.QFQDay))
	for _, row := range d.QFQDay {
		if len(row) < 6 {
			continue
		}
		var str [6]string
		ok := true
		for i := 0; i < 6; i++ {
			if err := json.Unmarshal(row[i], &str[i]); err != nil {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		c := types.Candle{Date: str[0]}
		c.Open, _ = strconv.ParseFloat(str[1], 64)
		c.Close, _ = strconv.ParseFloat(str[2], 64)
		c.High, _ = strconv.ParseFloat(str[3], 64)
		c.Low, _ = strconv.ParseFloat(str[4], 64)
		c.Volume, _ = strconv.ParseFloat(str[5], 64)
		candles = append(candles, c)
	}
	if candles == nil {
		candles = []types.Candle{}
	}

	// 报价：qt 数组固定索引（A股标准格式）
	quote := types.Quote{Code: code}
	if arr, ok := d.QT[key]; ok && len(arr) > 35 {
		atof := func(i int) float64 { v, _ := strconv.ParseFloat(arr[i], 64); return v }
		quote.Name = arr[1]
		quote.Price = atof(3)      // 最新价
		quote.PrevClose = atof(4)  // 昨收
		quote.Open = atof(5)       // 今开
		quote.Volume = atof(6)     // 成交量 手
		quote.Change = atof(31)    // 涨跌额
		quote.ChangePct = atof(32) // 涨跌幅 %
		quote.High = atof(33)      // 最高
		quote.Low = atof(34)       // 最低
	}

	return &types.Kline{Quote: quote, Candles: candles}, nil
}

// mktCode 市场 → 腾讯代码前缀（沪/深/北）
func mktCode(market string) (string, error) {
	switch strings.ToUpper(market) {
	case "沪A", "SH":
		return "sh", nil
	case "深A", "SZ":
		return "sz", nil
	case "北A", "BJ":
		return "bj", nil
	}
	return "", fmt.Errorf("不支持的市场: %s", market)
}

func doGet(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", browserUA)
	client := &http.Client{Timeout: 5 * time.Second}
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
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	return body, nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
