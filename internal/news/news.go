// Package news dao 层：相关资讯数据访问（东财资讯搜索，免登录公开接口）
package news

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"zhihudp/internal/types"
)

const browserUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"

// emTag 清洗东财返回的 <em>...</em> 高亮标签
var emTag = regexp.MustCompile(`</?em>`)

// GetNews 按关键词搜索相关资讯（东财，免登录）
// keyword: 股票名称（如 贵州茅台）；count: 条数，clamp [1,10]
func GetNews(ctx context.Context, keyword string, count int) ([]types.NewsItem, error) {
	if keyword == "" {
		return nil, fmt.Errorf("keyword 不能为空")
	}
	if count < 1 {
		count = 1
	}
	if count > 10 {
		count = 10
	}

	param := map[string]any{
		"uid":     "",
		"keyword": keyword,
		"type":    []string{"cmsArticleWebOld"},
		"client":  "web", "clientType": "web", "clientVersion": "curr",
		"param": map[string]any{
			"cmsArticleWebOld": map[string]any{
				"searchScope": "default", "sort": "default",
				"pageIndex": 1, "pageSize": count,
				"preTag": "<em>", "postTag": "</em>",
			},
		},
	}
	paramJSON, err := json.Marshal(param)
	if err != nil {
		return nil, fmt.Errorf("构造参数失败: %w", err)
	}

	u := "https://search-api-web.eastmoney.com/search/jsonp?cb=cb&param=" + url.QueryEscape(string(paramJSON))
	body, err := doGet(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("东财资讯请求失败: %w", err)
	}

	// 解析 JSONP：cb({...})
	raw := string(body)
	if len(raw) < 5 {
		return nil, fmt.Errorf("东财资讯响应异常")
	}
	raw = raw[3 : len(raw)-1] // 剥 cb( 与 )
	var resp struct {
		Code int `json:"code"`
		Result struct {
			Articles []struct {
				Date  string `json:"date"`
				Title string `json:"title"`
				Url   string `json:"url"`
			} `json:"cmsArticleWebOld"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, fmt.Errorf("东财资讯解析失败: %w", err)
	}

	items := make([]types.NewsItem, 0, len(resp.Result.Articles))
	for _, a := range resp.Result.Articles {
		items = append(items, types.NewsItem{
			Title:  emTag.ReplaceAllString(a.Title, ""),
			Url:    a.Url,
			Date:   a.Date,
			Source: "东方财富",
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
