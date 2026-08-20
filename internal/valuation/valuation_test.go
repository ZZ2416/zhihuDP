package valuation

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPercentile(t *testing.T) {
	cases := []struct {
		series []float64
		cur    float64
		want   float64
	}{
		{[]float64{10, 20, 30, 40, 50}, 15, 20},  // 低于 2/5 个
		{[]float64{10, 20, 30, 40, 50}, 50, 100}, // 最高 → 100
		{[]float64{10, 20, 30, 40, 50}, 10, 20},  // 最低 → 1/5
		{[]float64{10, 20, 30}, 25, 66.6666666},  // 中间
		{[]float64{}, 10, -1},                    // 空序列
	}
	for _, c := range cases {
		got := percentile(c.series, c.cur)
		if c.want == -1 && got != -1 {
			t.Errorf("percentile(%v,%v)=%v want -1", c.series, c.cur, got)
		} else if c.want >= 0 && (got < c.want-0.01 || got > c.want+0.01) {
			t.Errorf("percentile(%v,%v)=%v want %v", c.series, c.cur, got, c.want)
		}
	}
}

func TestSecidSuffix(t *testing.T) {
	if got := secidSuffix("沪A", "600519"); got != "SH" {
		t.Errorf("沪A → %q want SH", got)
	}
	if got := secidSuffix("深A", "000001"); got != "SZ" {
		t.Errorf("深A → %q want SZ", got)
	}
	if got := secidSuffix("", "300750"); got != "SZ" {
		t.Errorf("300750 → %q want SZ", got)
	}
}

// 腾讯 qt 解析（mock）
func TestFetchTx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 按真实 qt 字段位置构造：PE[39]、总市值[45]、PB[46]
		f := []string{"1", "x", "600519"}
		for len(f) < 39 {
			f = append(f, "0")
		}
		f = append(f, "19.90") // [39] PE
		for len(f) < 45 {
			f = append(f, "0")
		}
		f = append(f, "16203.56") // [45] 总市值
		f = append(f, "6.45")     // [46] PB
		for len(f) < 60 {
			f = append(f, "0")
		}
		_, _ = w.Write([]byte(`v_sh600519="` + strings.Join(f, "~") + `"`))
	}))
	defer srv.Close()
	old := txQtURL
	txQtURL = srv.URL + "/q="
	defer func() { txQtURL = old }()

	pb, mcap, pe, err := fetchTx(context.Background(), "沪A", "600519")
	if err != nil {
		t.Fatalf("fetchTx 失败: %v", err)
	}
	if pe != 19.90 {
		t.Errorf("PE=%v want 19.90", pe)
	}
	if pb != 6.45 {
		t.Errorf("PB=%v want 6.45", pb)
	}
	if mcap != 16203.56 {
		t.Errorf("市值=%v want 16203.56", mcap)
	}
}

// 东财 PE 序列解析 + 兜底（mock 双源）
func TestGetValuation(t *testing.T) {
	em := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 降序返回（最新在前），PE_TTM 序列 10/20/30/40/50，当前=50
		_, _ = w.Write([]byte(`{"result":{"data":[
			{"PE_TTM":50},{"PE_TTM":40},{"PE_TTM":30},{"PE_TTM":20},{"PE_TTM":10}]}}`))
	}))
	defer em.Close()
	tx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("v_sh600519=\"1~x~600519~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~20~0~0~0~0~0~6~0~0\""))
	}))
	defer tx.Close()

	oldEM, oldTX := emValueURL, txQtURL
	emValueURL = em.URL
	txQtURL = tx.URL + "/q="
	defer func() { emValueURL, txQtURL = oldEM, oldTX }()

	v, err := Get(context.Background(), "沪A", "600519")
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if v.PE != 50 || v.PEEntPercent != 100 {
		t.Errorf("PE=%v 分位=%v want 50/100", v.PE, v.PEEntPercent)
	}
}

// 亏损公司：当前 PE 为负 → 分位 -1，PE 显示真实负值
func TestGetValuationLossMaking(t *testing.T) {
	em := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 最新 PE=-8（亏损），历史有正有负
		_, _ = w.Write([]byte(`{"result":{"data":[
			{"PE_TTM":-8},{"PE_TTM":10},{"PE_TTM":15},{"PE_TTM":20},{"PE_TTM":12}]}}`))
	}))
	defer em.Close()
	tx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("v_sh600519=\"1~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0~0\""))
	}))
	defer tx.Close()
	oldEM, oldTX := emValueURL, txQtURL
	emValueURL = em.URL
	txQtURL = tx.URL + "/q="
	defer func() { emValueURL, txQtURL = oldEM, oldTX }()

	v, err := Get(context.Background(), "600519", "沪A")
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if v.PE != -8 {
		t.Errorf("亏损公司 PE 应为 -8，实际 %v", v.PE)
	}
	if v.PEEntPercent != -1 {
		t.Errorf("亏损公司分位应 -1，实际 %v", v.PEEntPercent)
	}
}

// 分位合法为 0（当前为历史最低）不应被误判为无分位
func TestPercentileZeroIsValid(t *testing.T) {
	series := []float64{10, 15, 20, 25, 30}
	if got := percentile(series, 10); got != 20 {
		t.Errorf("最低值分位应为 20（1/5），实际 %v", got)
	}
}
