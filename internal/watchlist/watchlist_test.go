package watchlist

import (
	"os"
	"path/filepath"
	"testing"

	"zhihudp/internal/types"
)

func TestAddRemoveList(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "wl.json"), 3)

	// 添加
	if _, err := s.Add("600519", "沪A"); err != nil {
		t.Fatalf("Add 失败: %v", err)
	}
	s.Add("000001", "深A")
	s.Add("300750", "深A")
	if got := len(s.List()); got != 3 {
		t.Fatalf("应有 3 条，实际 %d", got)
	}
	// 上限
	if _, err := s.Add("601318", "沪A"); err == nil {
		t.Fatal("超限应报错")
	}
	// 去重幂等
	if _, err := s.Add("600519", "沪A"); err != nil {
		t.Fatalf("重复添加应幂等: %v", err)
	}
	if got := len(s.List()); got != 3 {
		t.Fatalf("去重后应仍 3 条，实际 %d", got)
	}
	// 移除
	if err := s.Remove("000001"); err != nil {
		t.Fatalf("Remove 失败: %v", err)
	}
	if got := len(s.List()); got != 2 {
		t.Fatalf("移除后应 2 条，实际 %d", got)
	}
	// 移除不存在幂等
	if err := s.Remove("999999"); err != nil {
		t.Fatalf("移除不存在应幂等: %v", err)
	}
}

// 持久化：新实例重新加载
func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wl.json")
	s1 := New(path, 20)
	s1.Add("600519", "沪A")
	s1.Add("000001", "深A")

	s2 := New(path, 20)
	items := s2.List()
	if len(items) != 2 || items[0].Code != "600519" {
		t.Fatalf("重新加载失败: %+v", items)
	}
	_ = types.WatchItem{}
}

// 上限裁剪：文件超上限时加载截断
func TestLoadTruncate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wl.json")
	_ = os.WriteFile(path, []byte(`[{"code":"1","market":""},{"code":"2","market":""},{"code":"3","market":""},{"code":"4","market":""}]`), 0o644)
	s := New(path, 3)
	if got := len(s.List()); got != 3 {
		t.Fatalf("加载应截断为 3，实际 %d", got)
	}
}

// 上限满时 Add 报错
func TestAddFull(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "wl.json"), 2)
	s.Add("600519", "沪A")
	s.Add("000001", "深A")
	if _, err := s.Add("300750", "深A"); err == nil {
		t.Fatal("上限满应报错")
	}
}

// 空列表返回 []（非 nil）
func TestEmptyList(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "wl.json"), 20)
	if got := s.List(); got == nil || len(got) != 0 {
		t.Fatalf("空列表应为空 slice，实际 %v", got)
	}
}
