package finance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"zhihudp/internal/types"
)

func TestMarketPrefix(t *testing.T) {
	cases := []struct{ market, code, want string }{
		{"沪A", "600519", "SH"},
		{"深A", "000001", "SZ"},
		{"北交", "830799", "BJ"},
		{"", "600519", "SH"},
		{"", "000001", "SZ"},
		{"", "300750", "SZ"},
		{"", "830799", "BJ"},
		{"", "", "SH"},
	}
	for _, c := range cases {
		if got := marketPrefix(c.market, c.code); got != c.want {
			t.Errorf("marketPrefix(%q,%q)=%q want %q", c.market, c.code, got, c.want)
		}
	}
}

func TestRoundAndYi(t *testing.T) {
	if got := yi(92278072083.21); got != 922.78 {
		t.Errorf("yi 换算错误: %v", got)
	}
	if got := round2(1.300099475569); got != 1.3 {
		t.Errorf("round2 错误: %v", got)
	}
	if got := yi(0); got != 0 {
		t.Errorf("yi(0) 应为 0: %v", got)
	}
}

// 用 httptest mock 验证 datacenter 解析与兜底切换
func TestGetResultWithMock(t *testing.T) {
	dc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":{"data":[
			{"SECURITY_NAME_ABBR":"贵州茅台","REPORT_DATE":"2025-12-31 00:00:00","REPORT_DATE_NAME":"2025年报",
			 "TOTALOPERATEREVE":172100000000,"TOTALOPERATEREVETZ":1.3,"PARENTNETPROFIT":82300000000,
			 "PARENTNETPROFITTZ":-1.95,"EPSJB":32.5,"ROEJQ":32.53,"XSMLL":91.18,"XSJLL":47.8,
			 "ZCFZL":16.42,"JYXJLYYSR":0.8,"KCFJCXSYJLR":82000000000}
		]}}`))
	}))
	defer dc.Close()

	f10 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer f10.Close()

	// 临时替换 URL（测试注入）
	oldDC, oldF10 := datacenterURL, f10URL
	datacenterURL, f10URL = dc.URL, f10.URL
	defer func() { datacenterURL, f10URL = oldDC, oldF10 }()

	res, err := GetResult(context.Background(), "600519", "沪A")
	if err != nil {
		t.Fatalf("GetResult 失败: %v", err)
	}
	if res.Name != "贵州茅台" {
		t.Errorf("名称解析错误: %q", res.Name)
	}
	if len(res.Indicators) != 1 {
		t.Fatalf("指标数应为 1，实际 %d", len(res.Indicators))
	}
	it := res.Indicators[0]
	if it.Revenue != 1721 || it.NetProfit != 823 {
		t.Errorf("亿元归一化错误: 营收=%v 净利=%v", it.Revenue, it.NetProfit)
	}
	if it.ReportDate != "2025年报" || it.ReportDateFull != "2025-12-31" {
		t.Errorf("报告期解析错误: %+v", it)
	}
}

// 主接口失败 → 兜底 F10
func TestGetResultFallback(t *testing.T) {
	dc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer dc.Close()
	f10 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"SECURITY_NAME_ABBR":"贵州茅台","REPORT_DATE":"2024-12-31 00:00:00",
			"REPORT_DATE_NAME":"2024年报","TOTALOPERATEREVE":174100000000,"PARENTNETPROFIT":86200000000}]}`))
	}))
	defer f10.Close()

	oldDC, oldF10 := datacenterURL, f10URL
	datacenterURL, f10URL = dc.URL, f10.URL
	defer func() { datacenterURL, f10URL = oldDC, oldF10 }()

	res, err := GetResult(context.Background(), "600519", "沪A")
	if err != nil {
		t.Fatalf("兜底应成功: %v", err)
	}
	if !res.Degraded {
		t.Error("兜底时应置 Degraded=true")
	}
	if len(res.Indicators) != 1 || res.Indicators[0].ReportDate != "2024年报" {
		t.Errorf("兜底数据错误: %+v", res.Indicators)
	}
	_ = types.FinancialIndicator{} // 类型引用
}
