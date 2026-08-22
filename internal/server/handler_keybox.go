// 密钥箱 HTTP 处理器：GET /api/config/pubkey 下发公钥，POST /api/config/keys 接收加密密钥
package server

import (
	"encoding/json"
	"log"
	"net/http"
)

// pubKeyResponse 公钥下发响应
type pubKeyResponse struct {
	PublicKey string `json:"public_key"` // PEM 公钥
}

// handlePubKey GET /api/config/pubkey
// 返回 RSA 公钥 PEM，前端用它加密用户填写的 DeepSeek / 知乎密钥。
func (s *Server) handlePubKey(w http.ResponseWriter, r *http.Request) {
	if s.keyService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "密钥服务未启用"})
		return
	}
	writeJSON(w, http.StatusOK, pubKeyResponse{PublicKey: s.keyService.PublicKeyPEM()})
}

// updateKeysRequest 前端加密提交的密钥（base64 RSA-OAEP 密文，字段为空表示该项跳过）
type updateKeysRequest struct {
	DeepseekKey string `json:"deepseek_key_enc"` // 加密的 DeepSeek API Key
	ZhihuSecret string `json:"zhihu_secret_enc"` // 加密的知乎 Access Secret
}

// handleUpdateKeys POST /api/config/keys
// 解密用户密钥并热更新到运行中的配置；解密失败返回 400。
func (s *Server) handleUpdateKeys(w http.ResponseWriter, r *http.Request) {
	if s.keyService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "密钥服务未启用"})
		return
	}
	var req updateKeysRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体解析失败"})
		return
	}
	// 解密：交给 keybox 实现（私钥仅服务端持有）
	dk, err := s.keyService.DecryptOAEPBase64(req.DeepseekKey)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "DeepSeek 密钥解密失败"})
		return
	}
	zk, err := s.keyService.DecryptOAEPBase64(req.ZhihuSecret)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "知乎密钥解密失败"})
		return
	}
	if err := s.keyService.UpdateKeys(string(dk), string(zk)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	// 持久化密文到 config.yaml（只写 *_enc 字段）：重启后加载解密恢复，仓库/配置无明文
	persisted := true
	if err := s.keyService.PersistKeys(req.DeepseekKey, req.ZhihuSecret); err != nil {
		log.Printf("[keybox] 密文持久化失败: %v（密钥已热更新，重启后需重新上传）", err)
		persisted = false
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true, "persisted": persisted})
}
