// 视频资讯 HTTP 处理器：GET /api/video?keyword=&count=（B站，展示数据）
package server

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// handleVideo GET /api/video?keyword=贵州茅台&count=10
// 返回 B站相关视频（标题/播放量/发布时间/时长/UP主），前端按时间/播放量排序。
func (s *Server) handleVideo(w http.ResponseWriter, r *http.Request) {
	if s.video == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "视频服务未启用"})
		return
	}
	keyword := strings.TrimSpace(r.URL.Query().Get("keyword"))
	if keyword == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "keyword 不能为空"})
		return
	}
	count := 10
	if c := r.URL.Query().Get("count"); c != "" {
		n, err := strconv.Atoi(c)
		if err != nil || n < 1 || n > 20 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "count 需为 1-20 的整数"})
			return
		}
		count = n
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	start := time.Now()
	items, err := s.video.GetVideos(ctx, keyword, count)
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		log.Printf("[video] kw=%q 失败: %v 耗时=%dms", keyword, err, elapsed)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	log.Printf("[video] kw=%q count=%d 耗时=%dms", keyword, len(items), elapsed)
	writeJSON(w, http.StatusOK, items)
}
