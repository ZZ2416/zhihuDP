// 基本面数据 HTTP 处理器：GET /api/fundamental（评分数据，供前端/调试用）
package server

import (
	"context"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"zhihudp/internal/types"
)

// FundamentalProvider 基本面评分数据接口（由入口 fundamental 服务实现）
type FundamentalProvider interface {
	GetScore(ctx context.Context, code, market string) (*types.FundamentalResult, error)
}

// codeRe 股票代码：6 位数字
var codeRe = regexp.MustCompile(`^\d{6}$`)

// handleFundamental GET /api/fundamental?code=600519&market=沪A
// 返回四维评分 + 指标 + 估值（展示数据，不计配额）
func (s *Server) handleFundamental(w http.ResponseWriter, r *http.Request) {
	if s.fundamental == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "基本面服务未启用"})
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if !codeRe.MatchString(code) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code 需为 6 位数字股票代码"})
		return
	}
	market := strings.TrimSpace(r.URL.Query().Get("market"))

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	start := time.Now()
	res, err := s.fundamental.GetScore(ctx, code, market)
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		log.Printf("[fundamental] code=%s 失败: %v 耗时=%dms", code, err, elapsed)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	log.Printf("[fundamental] code=%s 总分=%d 耗时=%dms", code, res.Score.Total, elapsed)
	writeJSON(w, http.StatusOK, res)
}
