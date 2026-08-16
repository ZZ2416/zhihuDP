// 媒体播放处理器：GET /media/player（受保护播放页）+ GET /media/file（受保护视频流）
// 抖音式「禁止转载」：无/错 token 一律 403；播放页隐藏下载按钮、禁用右键/画中画/拖拽。
package server

import (
	"crypto/subtle"
	"html"
	"net/http"
	"os"
	"path/filepath"
)

// handleMediaPlayer GET /media/player?t=<token>&f=<文件名>
// 渲染受保护播放页（video 内嵌，禁下载/右键/画中画/拖拽）。
func (s *Server) handleMediaPlayer(w http.ResponseWriter, r *http.Request) {
	if !s.validMediaToken(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "无访问权限：token 缺失或无效"})
		return
	}
	name := filepath.Base(r.URL.Query().Get("f"))
	if name == "." || name == "" || !s.mediaFileExists(name) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "视频不存在"})
		return
	}
	token := html.EscapeString(r.URL.Query().Get("t"))
	// 播放页：视频直链带 token；禁下载/右键/画中画/拖拽
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!DOCTYPE html>
<html lang="zh-CN"><head><meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>zhihuDP · 视频播放</title>
<style>
  body { margin:0; min-height:100vh; display:flex; align-items:center; justify-content:center;
         background:#0d1117; font-family:-apple-system,"PingFang SC",sans-serif; }
  .box { width:min(960px, 94vw); }
  video { width:100%; border-radius:12px; background:#000; user-select:none; -webkit-user-drag:none; }
  .tip { text-align:center; color:#8b99a8; font-size:12px; margin-top:12px; }
</style></head>
<body oncontextmenu="return false" ondragstart="return false">
<div class="box">
  <video src="/media/file?t=` + token + `&f=` + html.EscapeString(name) + `"
         controls controlsList="nodownload noremoteplayback"
         disablepictureinpicture preload="metadata"
         oncontextmenu="return false" ondragstart="return false"></video>
  <div class="tip">🔒 受保护视频 · 禁止下载与转载</div>
</div>
</body></html>`))
}

// handleMediaFile GET /media/file?t=<token>&f=<文件名>
// 校验 token 后输出视频流（http.ServeFile 原生支持 Range，可拖动进度条）。
func (s *Server) handleMediaFile(w http.ResponseWriter, r *http.Request) {
	if !s.validMediaToken(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "无访问权限：token 缺失或无效"})
		return
	}
	name := filepath.Base(r.URL.Query().Get("f"))
	if name == "." || name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "文件名无效"})
		return
	}
	path := filepath.Join(s.mediaDir, name)
	http.ServeFile(w, r, path)
}

// validMediaToken 校验播放 token（常量时间比较防时序攻击）
func (s *Server) validMediaToken(r *http.Request) bool {
	if s.mediaDir == "" || s.mediaToken == "" {
		return false
	}
	got := r.URL.Query().Get("t")
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.mediaToken)) == 1
}

// mediaFileExists 媒体目录中是否存在该文件
func (s *Server) mediaFileExists(name string) bool {
	fi, err := os.Stat(filepath.Join(s.mediaDir, name))
	return err == nil && !fi.IsDir()
}
