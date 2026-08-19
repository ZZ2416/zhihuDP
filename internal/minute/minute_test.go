package minute

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecid(t *testing.T) {
	cases := []struct{ market, code, want string }{
		{"沪A", "600519", "1.600519"},
		{"深A", "000001", "0.000001"},
		{"", "600519", "1.600519"},
		{"", "000001", "0.000001"},
		{"", "300750", "0.300750"},
	}
	for _, c := range cases {
		if got := secid(c.market, c.code); got != c.want {
			t.Errorf("secid(%q,%q)=%q want %q", c.market, c.code, got, c.want)
		}
	}
}

func TestFetchEM(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"code":"600519","name":"贵州茅台","preClose":1297.99,
			"trends":["2026-08-19 09:30,1300.00,1300.00,1300.00,1300.00,315,40950000.00,1300.000",
			          "2026-08-19 09:31,1301.00,1301.00,1301.00,1301.00,500,65000000.00,1300.500"]}}`))
	}))
	defer srv.Close()
	old := emTrendsURL
	emTrendsURL = srv.URL
	defer func() { emTrendsURL = old }()

	res, err := fetchEM(context.Background(), "沪A", "600519")
	if err != nil {
		t.Fatalf("fetchEM 失败: %v", err)
	}
	if res.Name != "贵州茅台" || res.PreClose != 1297.99 {
		t.Errorf("基础字段错误: %+v", res)
	}
	if len(res.Points) != 2 {
		t.Fatalf("点数应为 2，实际 %d", len(res.Points))
	}
	p := res.Points[0]
	if p.Time != "09:30" || p.Price != 1300 || p.AvgPrice != 1300 || p.Volume != 315 {
		t.Errorf("点解析错误: %+v", p)
	}
}

func TestFetchTx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"msg":"","data":{"sh600519":{"data":{"date":"20260819",
			"data":["0930 1300.00 315 40950000.00","0931 1301.00 500 65000000.00"],
			"qt":{"prec":1297.99}}}}}`))
	}))
	defer srv.Close()
	old := txMinuteURL
	txMinuteURL = srv.URL
	defer func() { txMinuteURL = old }()

	res, err := fetchTx(context.Background(), "沪A", "600519")
	if err != nil {
		t.Fatalf("fetchTx 失败: %v", err)
	}
	if res.PreClose != 1297.99 || len(res.Points) != 2 {
		t.Errorf("腾讯解析错误: %+v", res)
	}
	if res.Points[0].Time != "09:30" || res.Points[0].Price != 1300 {
		t.Errorf("时间/价格解析错误: %+v", res.Points[0])
	}
	// 累计均价 = 累计额/累计量/100
	if res.Points[1].AvgPrice <= 0 {
		t.Errorf("累计均价应为正: %+v", res.Points[1])
	}
}
