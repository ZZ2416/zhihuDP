// Package hot dao 层：热门数据访问（东财涨幅榜：热门股票 / 行业板块，免登录公开接口）
package hot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"zhihudp/internal/types"
)

const browserUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"

// fsParam 按类型选择东财股票范围参数（m:90+t:2=行业板块；其余=沪深A股）
var fsParam = map[string]string{
	"stock":  "m:0+t:6,m:0+t:80,m:1+t:2,m:1+t:23,m:0+t:81+s:2048",
	"sector": "m:90+t:2",
}

// GetHot 获取热门榜（stock=热门股票 / sector=热门板块，按涨幅排序）
func GetHot(ctx context.Context, typ string, count int) ([]types.HotItem, error) {
	fs, ok := fsParam[typ]
	if !ok {
		return nil, fmt.Errorf("不支持的 type: %s（仅 stock / sector）", typ)
	}
	if count < 1 {
		count = 1
	}
	if count > 20 {
		count = 20
	}

	u := "https://push2.eastmoney.com/api/qt/clist/get"
	q := url.Values{}
	q.Set("pn", "1")
	q.Set("pz", strconv.Itoa(count))
	q.Set("po", "1")
	q.Set("np", "1")
	q.Set("fltt", "2")
	q.Set("invt", "2")
	q.Set("fid", "f3")
	q.Set("fs", fs)
	q.Set("fields", "f2,f3,f12,f14")

	body, err := doGet(ctx, u+"?"+q.Encode())
	if err != nil {
		return nil, fmt.Errorf("东财热门请求失败: %w", err)
	}
	var resp struct {
		Data struct {
			Diff []struct {
				F12 string  `json:"f12"` // 代码
				F14 string  `json:"f14"` // 名称
				F2  float64 `json:"f2"`  // 最新价
				F3  float64 `json:"f3"`  // 涨跌幅 %
			} `json:"diff"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("东财热门解析失败: %w", err)
	}

	items := make([]types.HotItem, 0, len(resp.Data.Diff))
	for _, d := range resp.Data.Diff {
		if d.F12 == "" || d.F14 == "" {
			continue
		}
		items = append(items, types.HotItem{
			Code:      d.F12,
			Name:      d.F14,
			Price:     d.F2,
			ChangePct: d.F3,
			Type:      typ,
		})
	}
	return items, nil
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
