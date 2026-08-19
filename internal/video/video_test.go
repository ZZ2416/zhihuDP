package video

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCleanTitle(t *testing.T) {
	got := cleanTitle(`<em class="keyword">贵州茅台</em>2026<em>中报</em>解读`)
	if got != "贵州茅台2026中报解读" {
		t.Errorf("清洗失败: %q", got)
	}
}

func TestFmtDuration(t *testing.T) {
	if fmtDuration(125) != "2:05" || fmtDuration(60) != "1:00" {
		t.Errorf("时长格式化错误")
	}
}

func TestMixinKey(t *testing.T) {
	// 32 字符 img/sub key（真实 nav 返回格式），预期值由同算法独立计算
	mk := mixinKeyFrom("aefce2615e2d8e2daefce2615e2d8e2d", "7c16d6e8c0f5a4b87c16d6e8c0f5a4b8")
	if mk != "b8ff6517d12dfc46d52ccefce8288e0e" {
		t.Errorf("mixinKey 算法错误: %q", mk)
	}
	// 短 key 不应 panic（越界保护）
	if got := mixinKeyFrom("short", "keys"); len(got) == 0 {
		t.Errorf("短 key 应返回非空")
	}
}

func TestGetVideosWithMock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "nav") {
			_, _ = w.Write([]byte(`{"code":0,"data":{"wbi_img":{"img_url":"https://i0.hdslb.com/bfs/wbi/aefce2615e2d8e2d.png","sub_url":"https://i0.hdslb.com/bfs/wbi/7c16d6e8c0f5a4b8.png"}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"message":"0","data":{"result":[
			{"title":"<em class=\"keyword\">贵州茅台</em>解读","bvid":"BV1xx","play":1180,"video_review":2,"duration":"1021","pubdate":1787020000,"author":"测试UP"}
		]}}`))
	}))
	defer srv.Close()
	oldNav, oldSearch := navURL, searchURL
	navURL, searchURL = srv.URL+"/nav", srv.URL+"/search"
	defer func() { navURL, searchURL = oldNav, oldSearch }()

	items, err := GetVideos(context.Background(), "贵州茅台", 5)
	if err != nil {
		t.Fatalf("GetVideos 失败: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("结果数应为 1，实际 %d", len(items))
	}
	v := items[0]
	if v.Title != "贵州茅台解读" || v.Play != 1180 || v.Bvid != "BV1xx" {
		t.Errorf("字段解析错误: %+v", v)
	}
	if !strings.HasPrefix(v.Url, "https://www.bilibili.com/video/") || v.Duration != "17:01" {
		t.Errorf("链接/时长错误: %+v", v)
	}
}
