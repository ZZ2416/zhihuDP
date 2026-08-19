// Package video dao 层：B站视频资讯（wbi 签名 + 搜索接口，实测 2026-08-19）
// 选型：抖音 web 搜索需 a_bogus 签名（反爬强/易失效/法律风险），B站 wbi 签名公开简单、字段齐全。
package video

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"zhihudp/internal/types"
)

const browserUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"

var (
	navURL    = "https://api.bilibili.com/x/web-interface/nav"             // 获取 wbi 密钥
	searchURL = "https://api.bilibili.com/x/web-interface/wbi/search/type" // 搜索
	// mixinKeyEncTab wbi 混合密钥重排表（B站公开算法）
	mixinKeyEncTab = []int{46, 47, 18, 2, 53, 8, 23, 32, 15, 50, 10, 31, 58, 3, 45, 35, 27, 43, 5, 49, 33, 9, 42, 19, 29, 28, 14, 39, 12, 38, 41, 13, 37, 48, 7, 16, 24, 55, 40, 61, 26, 17, 0, 1, 60, 51, 30, 4, 22, 25, 54, 21, 56, 59, 6, 63, 57, 62, 11, 36, 20, 34, 44, 52}
)

// wbiKeys wbi 密钥缓存（nav 返回的 img/sub key，2 小时有效）
type wbiKeys struct {
	mu       sync.Mutex
	mixinKey string
	fetched  time.Time
}

var keysCache wbiKeys

// GetVideos 搜索 B站视频（按关键词；返回按接口相关度，由前端按时间/播放量重排）
func GetVideos(ctx context.Context, keyword string, count int) ([]types.VideoItem, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, fmt.Errorf("关键词不能为空")
	}
	if count < 1 {
		count = 5
	}
	if count > 20 {
		count = 20
	}
	mk, err := mixinKey(ctx)
	if err != nil {
		return nil, err
	}
	params := map[string]string{
		"keyword":     keyword,
		"search_type": "video",
		"page":        "1",
		"page_size":   strconv.Itoa(count),
	}
	qs := wbiSign(params, mk)
	body, err := doGet(ctx, searchURL+"?"+qs)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Result []struct {
				Title       string `json:"title"`
				Bvid        string `json:"bvid"`
				Pic         string `json:"pic"`
				Play        int64  `json:"play"`
				VideoReview int64  `json:"video_review"`
				Duration    string `json:"duration"` // 秒（字符串）
				Pubdate     int64  `json:"pubdate"`  // 时间戳
				Author      string `json:"author"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("B站搜索解析失败: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("B站搜索 code=%d %s", resp.Code, resp.Message)
	}
	items := make([]types.VideoItem, 0, len(resp.Data.Result))
	for _, v := range resp.Data.Result {
		if v.Bvid == "" {
			continue
		}
		dur := v.Duration
		if sec, err := strconv.Atoi(v.Duration); err == nil {
			dur = fmtDuration(sec) // 秒数格式
		} // 否则 B站返回 "mm:ss" 原样使用
		items = append(items, types.VideoItem{
			Title:       cleanTitle(v.Title),
			Url:         "https://www.bilibili.com/video/" + v.Bvid,
			Pic:         absURL(v.Pic),
			Bvid:        v.Bvid,
			Play:        v.Play,
			Danmaku:     v.VideoReview,
			Duration:    dur,
			PublishTime: time.Unix(v.Pubdate, 0).Format("2006-01-02 15:04"),
			Author:      v.Author,
		})
	}
	return items, nil
}

// ---- wbi 签名（B站公开算法） ----

// mixinKey 获取混合密钥（缓存 2 小时）
func mixinKey(ctx context.Context) (string, error) {
	keysCache.mu.Lock()
	defer keysCache.mu.Unlock()
	if keysCache.mixinKey != "" && time.Since(keysCache.fetched) < 2*time.Hour {
		return keysCache.mixinKey, nil
	}
	body, err := doGet(ctx, navURL)
	if err != nil {
		return "", fmt.Errorf("获取 wbi 密钥失败: %w", err)
	}
	var nav struct {
		Code int `json:"code"`
		Data struct {
			WbiImg struct {
				ImgURL string `json:"img_url"`
				SubURL string `json:"sub_url"`
			} `json:"wbi_img"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &nav); err != nil {
		return "", fmt.Errorf("nav 解析失败: %v", err)
	}
	// 注意：nav 未登录时 code=-101，但 wbi_img 密钥仍正常返回，仅校验密钥非空即可
	if nav.Data.WbiImg.ImgURL == "" || nav.Data.WbiImg.SubURL == "" {
		return "", fmt.Errorf("nav 未返回 wbi_img（code=%d）", nav.Code)
	}
	imgKey := fileKey(nav.Data.WbiImg.ImgURL)
	subKey := fileKey(nav.Data.WbiImg.SubURL)
	mk := mixinKeyFrom(imgKey, subKey)
	keysCache.mixinKey = mk
	keysCache.fetched = time.Now()
	return mk, nil
}

// fileKey 取 URL 文件名去扩展名（.../xxx.png → xxx）
func fileKey(u string) string {
	s := u[strings.LastIndex(u, "/")+1:]
	return strings.TrimSuffix(s, ".png")
}

// mixinKeyFrom 混合 img_key 与 sub_key，按重排表取前 32 位（短输入防御）
func mixinKeyFrom(imgKey, subKey string) string {
	raw := imgKey + subKey
	var b strings.Builder
	for _, i := range mixinKeyEncTab {
		if i < len(raw) {
			b.WriteByte(raw[i])
		}
	}
	out := b.String()
	if len(out) > 32 {
		out = out[:32]
	}
	return out
}

// wbiSign 参数签名：加 wts → 排序 → urlencode → md5 拼接
func wbiSign(params map[string]string, mixinKey string) string {
	params["wts"] = strconv.FormatInt(time.Now().Unix(), 10)
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sortStrings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString("&")
		}
		b.WriteString(url.QueryEscape(k))
		b.WriteString("=")
		b.WriteString(url.QueryEscape(params[k]))
	}
	sum := md5.Sum([]byte(b.String() + mixinKey))
	params["w_rid"] = hex.EncodeToString(sum[:])
	vals := url.Values{}
	for k, v := range params {
		vals.Set(k, v)
	}
	return vals.Encode()
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// ---- 工具 ----

var emTag = regexp.MustCompile(`<[^>]+>`)

// absURL 补全封面 URL（//i0.hdslb.com/... → https://i0.hdslb.com/...）
func absURL(u string) string {
	if strings.HasPrefix(u, "//") {
		return "https:" + u
	}
	return u
}

// cleanTitle 去除 <em class="keyword"> 等高亮标签
func cleanTitle(s string) string {
	return emTag.ReplaceAllString(s, "")
}

// fmtDuration 秒 → mm:ss
func fmtDuration(sec int) string {
	if sec < 0 {
		sec = 0
	}
	m := sec / 60
	s := sec % 60
	return fmt.Sprintf("%d:%02d", m, s)
}

func doGet(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Referer", "https://www.bilibili.com/")
	client := &http.Client{Timeout: 8 * time.Second}
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
