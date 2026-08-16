// Package zhihu dao 层：知乎开放平台数据访问（Bearer + X-Request-Timestamp 鉴权）
package zhihu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"zhihudp/internal/config"
	"zhihudp/internal/types"
)

// SearchResponse 知乎 zhihu_search 响应（字段名以实际 JSON 为准，PascalCase）
type SearchResponse struct {
	Code    int    `json:"Code"`
	Message string `json:"Message"`
	Data    struct {
		HasMore bool   `json:"HasMore"`
		Items   []Item `json:"Items"`
	} `json:"Data"`
}

// Item 单条搜索结果
type Item struct {
	Title       string `json:"Title"`
	Url         string `json:"Url"`
	ContentText string `json:"ContentText"`
	AuthorName  string `json:"AuthorName"`
	VoteUpCount int    `json:"VoteUpCount"`
	EditTime    int64  `json:"EditTime"`
}

// Client 知乎开放平台客户端
type Client struct {
	cfg config.ZhihuConfig
}

// New 创建客户端
func New(cfg config.ZhihuConfig) *Client {
	return &Client{cfg: cfg}
}

// Search 调用 zhihu_search 开放接口
func (c *Client) Search(ctx context.Context, query string, count int) (*SearchResponse, error) {
	if c.cfg.AccessSecret == "" {
		return nil, errors.New("未配置 zhihu.access_secret（知乎 Bearer token）")
	}
	if count < 1 {
		count = 1
	}
	if count > 10 {
		count = 10
	}

	endpoint := c.endpoint()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("Query", query)
	q.Set("Count", strconv.Itoa(count))
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Authorization", "Bearer "+c.cfg.AccessSecret)
	req.Header.Set("X-Request-Timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 zhihu_search 失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 zhihu_search 响应失败: %w", err)
	}

	// 非 2xx 透传响应体
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("zhihu_search 非 2xx: %d, body: %s", resp.StatusCode, truncate(string(body), 500))
	}

	var sr SearchResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, fmt.Errorf("zhihu_search 响应解析失败: %w", err)
	}

	// Code != 0 透传 Message
	if sr.Code != 0 {
		return nil, fmt.Errorf("zhihu_search Code=%d Message=%s", sr.Code, sr.Message)
	}

	// 空结果返回空数组（非 nil），便于降级逻辑判断
	if sr.Data.Items == nil {
		sr.Data.Items = []Item{}
	}
	return &sr, nil
}

// HotList 知乎热榜（每 3h 更新，主数据源热门数据）
func (c *Client) HotList(ctx context.Context, count int) ([]types.ZhihuHotItem, error) {
	if c.cfg.AccessSecret == "" {
		return nil, errors.New("未配置 zhihu.access_secret（知乎 Bearer token）")
	}
	if count < 1 {
		count = 1
	}
	if count > 30 {
		count = 30
	}

	base := strings.TrimRight(c.cfg.OpenAPIBaseURL, "/")
	if base == "" {
		base = "https://developer.zhihu.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/content/hot_list", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.AccessSecret)
	req.Header.Set("X-Request-Timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 hot_list 失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 hot_list 响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("hot_list 非 2xx: %d, body: %s", resp.StatusCode, truncate(string(body), 500))
	}

	var sr struct {
		Code    int    `json:"Code"`
		Message string `json:"Message"`
		Data    struct {
			Total int `json:"Total"`
			Items []struct {
				Title        string `json:"Title"`
				Url          string `json:"Url"`
				Summary      string `json:"Summary"`
				ThumbnailUrl string `json:"ThumbnailUrl"`
			} `json:"Items"`
		} `json:"Data"`
	}
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, fmt.Errorf("hot_list 响应解析失败: %w", err)
	}
	if sr.Code != 0 {
		return nil, fmt.Errorf("hot_list Code=%d Message=%s", sr.Code, sr.Message)
	}

	items := make([]types.ZhihuHotItem, 0, len(sr.Data.Items))
	for _, it := range sr.Data.Items {
		if it.Title == "" {
			continue
		}
		items = append(items, types.ZhihuHotItem{
			Title:     it.Title,
			Url:       it.Url,
			Summary:   truncate(it.Summary, 120),
			Thumbnail: it.ThumbnailUrl,
		})
	}
	if len(items) > count {
		items = items[:count]
	}
	return items, nil
}

// endpoint 端点解析优先级：完整 URL > BaseURL + 路径 > 默认
func (c *Client) endpoint() string {
	if c.cfg.SearchURL != "" {
		return c.cfg.SearchURL
	}
	base := strings.TrimRight(c.cfg.OpenAPIBaseURL, "/")
	if base == "" {
		base = "https://developer.zhihu.com"
	}
	return base + "/api/v1/content/zhihu_search"
}

// truncate 按 rune 安全截断，避免切断 UTF-8 字符
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
