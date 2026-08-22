package chain

import (
	"context"
	"testing"

	"zhihudp/internal/types"
)

// parseChain 是 agent 包的内部函数——通过 agent 包公开的 parse 测试？不，直接测 Service。
// 这里测 Service.Generate（注入 mock Resolve/Generate）

func TestGenerateValidatesCompanies(t *testing.T) {
	svc := New(Deps{
		Resolve: func(_ context.Context, q string) (*types.StockInfo, error) {
			if q == "600519" {
				return &types.StockInfo{Code: "600519", Name: "贵州茅台", Market: "沪A"}, nil
			}
			return nil, types.ErrStockNotFound // 无效代码
		},
		Generate: func(_ context.Context, _, _ string) (*types.ChainResult, error) {
			return &types.ChainResult{
				Industry: "白酒产业链",
				Nodes: []types.ChainNode{
					{ID: "n1", Name: "上游-粮食", Stage: "上游", Desc: "粮食"},
					{ID: "n2", Name: "中游-酒企", Stage: "中游", Desc: "酒企"},
				},
				Edges: []types.ChainEdge{{From: "n1", To: "n2"}},
				Companies: map[string][]types.ChainCompany{
					"n1": {{Code: "600519", Name: "贵州茅台"}, {Code: "999999", Name: "假公司"}},
					"n2": {{Code: "600519", Name: "贵州茅台"}},
				},
			}, nil
		},
	})
	res, err := svc.Generate(context.Background(), "600519", "沪A")
	if err != nil {
		t.Fatalf("Generate 失败: %v", err)
	}
	// 999999 应被过滤，600519 保留并规范名
	cs := res.Companies["n1"]
	if len(cs) != 1 || cs[0].Code != "600519" {
		t.Fatalf("无效厂商应过滤: %+v", cs)
	}
	if !res.Degraded {
		t.Error("有厂商被过滤应置 Degraded")
	}
}

func TestValidateCode(t *testing.T) {
	cases := []struct {
		code string
		want bool
	}{
		{"600519", true}, {"000001", true}, {"60051", false},
		{"abc123", false}, {"6005190", false}, {"", false},
	}
	for _, c := range cases {
		if got := ValidateCode(c.code); got != c.want {
			t.Errorf("ValidateCode(%q)=%v want %v", c.code, got, c.want)
		}
	}
}
