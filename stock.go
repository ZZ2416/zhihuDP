package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrEmptyQuery    = errors.New("empty query")
	ErrStockNotFound = errors.New("stock not found")
)

// eastmoney 接口响应（实测：searchapi.eastmoney.com/api/suggest/get）
type eastmoneyResp struct {
	QuotationCodeTable struct {
		Data []struct {
			Code             string `json:"Code"`
			Name             string `json:"Name"`
			Classify         string `json:"Classify"` // AStock / Fund / Bond ...
			SecurityTypeName string `json:"SecurityTypeName"` // 沪A / 深A / 北A
		} `json:"Data"`
	} `json:"QuotationCodeTable"`
}

// tencent 接口响应（实测：smartbox.gtimg.cn/s3/?v=2&q=...&t=all）
// 返回格式非标准 JSON：v_hint="sh~600519~贵州茅台~gzmt~GP-A^sz~...^hk~...";
//   - 无匹配：v_hint="N";
//   - 多个结果以 ^ 分隔（A股^港股^美股），优先取 A 股前缀（sh/sz/bj）
func resolveViaTencent(ctx context.Context, query string) (*StockInfo, error) {
	u := "https://smartbox.gtimg.cn/s3/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("v", "2")
	q.Set("q", query)
	q.Set("t", "all")
	req.URL.RawQuery = q.Encode()

	body, err := doGet(req, 5*time.Second)
	if err != nil {
		return nil, err
	}

	// 手工解析 v_hint="..."; 格式
	raw := strings.TrimSpace(string(body))
	raw = strings.TrimPrefix(raw, "v_hint=")
	raw = strings.TrimSuffix(raw, ";")
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, `"`) || !strings.HasSuffix(raw, `"`) {
		return nil, fmt.Errorf("腾讯响应格式异常: %q", truncate(string(body), 200))
	}
	var hint string
	if err := json.Unmarshal([]byte(raw), &hint); err != nil { // 借 JSON 解析处理 \u 转义
		return nil, fmt.Errorf("腾讯 v_hint 解析失败: %w", err)
	}
	if hint == "" || hint == "N" {
		return nil, ErrStockNotFound
	}

	// sh~600519~贵州茅台~gzmt~GP-A；多条目 ^ 分隔，优先 A 股前缀
	for _, e := range strings.Split(hint, "^") {
		parts := strings.Split(e, "~")
		if len(parts) >= 3 {
			market := map[string]string{"sh": "沪A", "sz": "深A", "bj": "北A"}[parts[0]]
			if market != "" {
				return &StockInfo{Code: parts[1], Name: parts[2], Market: market}, nil
			}
		}
	}
	// 无 A 股条目时取第一个有效条目
	if first := strings.Split(hint, "^")[0]; first != "" {
		parts := strings.Split(first, "~")
		if len(parts) >= 3 {
			return &StockInfo{Code: parts[1], Name: parts[2], Market: parts[0]}, nil
		}
	}
	return nil, ErrStockNotFound
}

// ResolveStock 股票名称/代码 → 结构化股票信息；东财主选，失败/无匹配自动切腾讯兜底
func ResolveStock(ctx context.Context, query string) (*StockInfo, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, ErrEmptyQuery
	}

	// 1) 东财主选
	info, err := resolveViaEastmoney(ctx, query)
	if err == nil {
		return info, nil
	}
	eastErr := err

	// 2) 腾讯兜底
	info, err = resolveViaTencent(ctx, query)
	if err == nil {
		return info, nil
	}

	// 两个源都确认无此股票 → 语义化为 ErrStockNotFound（HTTP 404）
	if errors.Is(eastErr, ErrStockNotFound) && errors.Is(err, ErrStockNotFound) {
		return nil, ErrStockNotFound
	}
	return nil, fmt.Errorf("东财: %v；腾讯: %v", eastErr, err)
}

func resolveViaEastmoney(ctx context.Context, query string) (*StockInfo, error) {
	u := "https://searchapi.eastmoney.com/api/suggest/get"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("input", query)
	q.Set("type", "14") // 14=股票
	q.Set("count", "5")
	req.URL.RawQuery = q.Encode()

	body, err := doGet(req, 5*time.Second)
	if err != nil {
		return nil, err
	}

	var resp eastmoneyResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("东财响应解析失败: %w", err)
	}

	// 优先 A 股，其次任意匹配
	var fallback *StockInfo
	for _, d := range resp.QuotationCodeTable.Data {
		info := &StockInfo{Code: d.Code, Name: d.Name, Market: d.SecurityTypeName}
		if d.Classify == "AStock" {
			return info, nil
		}
		if fallback == nil && d.Code != "" {
			fallback = info
		}
	}
	if fallback != nil {
		return fallback, nil
	}
	return nil, ErrStockNotFound
}

func doGet(req *http.Request, timeout time.Duration) ([]byte, error) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
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
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
