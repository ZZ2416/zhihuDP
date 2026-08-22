package agent

import "testing"

func TestParseChain(t *testing.T) {
	// 正常 JSON
	ok := `{"industry":"白酒产业链","nodes":[{"id":"n1","name":"上游-粮食","stage":"上游","desc":"粮食"},
		{"id":"n2","name":"中游-酒企","stage":"中游","desc":"酒企"},{"id":"x","name":"坏节点","stage":"未知"}],
		"edges":[{"from":"n1","to":"n2"},{"from":"n1","to":"nope"}],
		"companies":{"n1":[{"code":"600519","name":"贵州茅台"}],"nope":[{"code":"000001","name":"X"}]}}`
	res, err := parseChain(ok)
	if err != nil {
		t.Fatalf("parseChain 失败: %v", err)
	}
	if res.Industry != "白酒产业链" {
		t.Errorf("industry=%q", res.Industry)
	}
	// 非法 stage 节点被过滤
	if len(res.Nodes) != 2 {
		t.Errorf("nodes 应为 2（坏节点过滤），实际 %d", len(res.Nodes))
	}
	// 无效 edge 过滤
	if len(res.Edges) != 1 {
		t.Errorf("edges 应为 1（无效端点过滤），实际 %d", len(res.Edges))
	}
	// 无效 companies 键删除
	if _, ok := res.Companies["nope"]; ok {
		t.Error("无效 companies 键应删除")
	}
	// markdown 围栏
	md := "```json\n" + ok + "\n```"
	res2, err := parseChain(md)
	if err != nil || res2 == nil {
		t.Errorf("markdown 围栏应可解析: %v", err)
	}
	// 非法 JSON
	if _, err := parseChain("not json"); err == nil {
		t.Error("非法 JSON 应报错")
	}
	// 空 nodes
	if _, err := parseChain(`{"nodes":[],"edges":[]}`); err == nil {
		t.Error("空 nodes 应报错")
	}
}
