// 产业链 HTTP 处理器：POST /api/chain（AI 生成产业链流程图，计配额）
package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"zhihudp/internal/chain"
	"zhihudp/internal/types"
)

// ChainProvider 产业链服务接口
type ChainProvider interface {
	Generate(ctx context.Context, code, market string) (*types.ChainResult, error)
}

// handleChain POST /api/chain {"code":"600519","market":"沪A"}
func (s *Server) handleChain(w http.ResponseWriter, r *http.Request) {
	if s.chainProvider == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "产业链服务未启用"})
		return
	}
	var req struct {
		Code   string `json:"code"`
		Market string `json:"market"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体必须是 JSON"})
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	if !chain.ValidateCode(req.Code) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code 需为 6 位数字股票代码"})
		return
	}

	// 会话配额（与 ask/chat 等共享）
	if _, ok := s.consumeQuota(w, r); !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "本次访问的 API 调用次数已用完（20 次），请重新打开页面"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	start := time.Now()
	res, err := s.chainProvider.Generate(ctx, req.Code, req.Market)
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		log.Printf("[chain] code=%s 失败: %v 耗时=%dms", req.Code, err, elapsed)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "产业链生成失败，请稍后重试"})
		return
	}
	log.Printf("[chain] code=%s 环节=%d 厂商=%d 耗时=%dms", req.Code, len(res.Nodes), len(res.Companies), elapsed)
	writeJSON(w, http.StatusOK, res)
}
