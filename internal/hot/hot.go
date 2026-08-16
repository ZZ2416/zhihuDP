// Package hot dao 层：热门数据访问（腾讯排行：热门股票 / 行业板块 / 板块成分股）
// 选型：腾讯接口实测稳定（与 K线同源），东财 push2 曾被限流故弃用
package hot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"zhihudp/internal/types"
)

const browserUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"

const (
	rankURL = "https://proxy.finance.qq.com/cgi/cgi-bin/rank/hs/getBoardRankList" // 股票/板块成分排行
	boardURL = "https://proxy.finance.qq.com/ifzqgtimg/appstock/app/mktHs/rank"   // 行业板块排行
)

// GetHot 获取热门榜（stock=热门股票 / sector=热门板块涨幅榜 / sector_fall=暴跌板块跌幅榜）
func GetHot(ctx context.Context, typ string, count int) ([]types.HotItem, error) {
	if count < 1 {
		count = 1
	}
	if count > 20 {
		count = 20
	}
	switch typ {
	case "stock":
		return fetchRankList(ctx, "aStock", count, "stock")
	case "sector":
		return fetchSectorList(ctx, count, true)
	case "sector_fall":
		return fetchSectorList(ctx, count, false)
	}
	return nil, fmt.Errorf("不支持的 type: %s（仅 stock / sector / sector_fall）", typ)
}

// GetSectorStocks 获取板块成分股（code=板块代码，如 pt01801712）
func GetSectorStocks(ctx context.Context, code string, count int) ([]types.HotItem, error) {
	if code == "" {
		return nil, fmt.Errorf("板块代码不能为空")
	}
	return fetchRankList(ctx, code, count, "stock")
}

// fetchRankList 腾讯股票/成分股排行（按成交额降序 = 市场最活跃/热门）
func fetchRankList(ctx context.Context, boardCode string, count int, typ string) ([]types.HotItem, error) {
	q := url.Values{}
	q.Set("board_code", boardCode)
	q.Set("sort_type", "turnover") // 成交额降序（热门活跃股；腾讯无涨幅排序，涨幅榜会被新股/妖股霸榜）
	q.Set("direct", "down")
	q.Set("offset", "0")
	q.Set("count", strconv.Itoa(count))

	body, err := doGet(ctx, rankURL+"?"+q.Encode())
	if err != nil {
		return nil, fmt.Errorf("腾讯排行请求失败: %w", err)
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			RankList []struct {
				Code string `json:"code"` // 带市场前缀，如 sh600519
				Name string `json:"name"`
				ZXJ  string `json:"zxj"` // 最新价（字符串）
				ZDF  string `json:"zdf"` // 涨跌幅 %（字符串）
			} `json:"rank_list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("腾讯排行解析失败: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("腾讯排行 code=%d", resp.Code)
	}

	items := make([]types.HotItem, 0, len(resp.Data.RankList))
	for _, d := range resp.Data.RankList {
		if d.Code == "" || d.Name == "" {
			continue
		}
		price, _ := strconv.ParseFloat(d.ZXJ, 64)
		chg, _ := strconv.ParseFloat(d.ZDF, 64)
		items = append(items, types.HotItem{
			Code:      stripMarketPrefix(d.Code),
			Name:      d.Name,
			Price:     price,
			ChangePct: chg,
			Type:      typ,
		})
	}
	return items, nil
}

// fetchSectorList 腾讯行业板块排行：一次拉取全量板块（l=200，实测约 124 个行业板块），
// 本地按平均涨幅排序取前 count。
//   - ascend=true  → 涨幅榜（热门板块，涨幅降序取前 count）
//   - ascend=false → 暴跌板块（跌幅榜，涨幅升序即跌幅最大取前 count）
//
// 注：接口固定按涨幅降序返回，`direct`/`sort` 等排序参数实测无效，故采用本地排序。
func fetchSectorList(ctx context.Context, count int, ascend bool) ([]types.HotItem, error) {
	all, err := fetchAllSectors(ctx)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(all, func(i, j int) bool {
		if ascend {
			return all[i].ChangePct > all[j].ChangePct
		}
		return all[i].ChangePct < all[j].ChangePct
	})
	if len(all) > count {
		all = all[:count]
	}
	return all, nil
}

// fetchAllSectors 拉取全量行业板块（腾讯 mktHs/rank）
func fetchAllSectors(ctx context.Context) ([]types.HotItem, error) {
	q := url.Values{}
	q.Set("l", "200") // 超过实际板块数，接口返回全量（实测 124 个行业板块）
	q.Set("p", "1")
	q.Set("t", "01/averatio")

	body, err := doGet(ctx, boardURL+"?"+q.Encode())
	if err != nil {
		return nil, fmt.Errorf("腾讯板块请求失败: %w", err)
	}
	var resp struct {
		Code int `json:"code"`
		Data []struct {
			BDCode string `json:"bd_code"` // 板块代码，如 pt01801712
			BDName string `json:"bd_name"`
			BDZXJ  string `json:"bd_zxj"` // 板块指数点位（字符串）
			BDZDF  string `json:"bd_zdf"` // 涨幅 %（字符串）
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("腾讯板块解析失败: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("腾讯板块 code=%d", resp.Code)
	}

	items := make([]types.HotItem, 0, len(resp.Data))
	for _, d := range resp.Data {
		if d.BDCode == "" || d.BDName == "" {
			continue
		}
		price, _ := strconv.ParseFloat(d.BDZXJ, 64)
		chg, _ := strconv.ParseFloat(d.BDZDF, 64)
		items = append(items, types.HotItem{
			Code:      d.BDCode,
			Name:      d.BDName,
			Price:     price,
			ChangePct: chg,
			Type:      "sector",
		})
	}
	return items, nil
}

// stripMarketPrefix 去掉市场前缀（sh600519 → 600519）
func stripMarketPrefix(code string) string {
	if len(code) > 2 {
		switch code[:2] {
		case "sh", "sz", "bj":
			return code[2:]
		}
	}
	return code
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

