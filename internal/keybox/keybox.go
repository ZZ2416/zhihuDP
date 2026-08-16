// Package keybox 密钥箱：RSA 公私钥加解密。
// 用途：前端弹窗让用户配置自己的 DeepSeek / 知乎密钥时，用服务端下发的公钥加密传输，
// 服务端持私钥解密 —— 密钥明文不出用户浏览器、不落日志，防止被他人截获利用。
package keybox

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"sync"
)

// KeyBox 持有一对 RSA 密钥：公钥下发给前端加密，私钥仅服务端内存持有。
// 每次启动随机生成新密钥对（无需持久化：解密发生在进程内，重启后前端重新拉取公钥即可）。
type KeyBox struct {
	mu   sync.RWMutex
	priv *rsa.PrivateKey
	pub  string // PEM 公钥，缓存供 GET /api/config/pubkey
}

// New 生成一对 2048 位 RSA 密钥。
func New() (*KeyBox, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("生成 RSA 密钥对失败: %w", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("编码公钥失败: %w", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	return &KeyBox{priv: priv, pub: string(pubPEM)}, nil
}

// PublicKeyPEM 返回公钥 PEM（可安全下发给前端）。
func (k *KeyBox) PublicKeyPEM() string {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.pub
}

// DecryptOAEPBase64 用私钥解密前端提交的 base64(RSA-OAEP 密文)。
// 返回明文；若为空字符串输入返回空（表示该密钥用户未填写，跳过更新）。
func (k *KeyBox) DecryptOAEPBase64(b64 string) ([]byte, error) {
	if b64 == "" {
		return nil, nil
	}
	ciphertext, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("解码密文失败: %w", err)
	}
	k.mu.RLock()
	defer k.mu.RUnlock()
	plain, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, k.priv, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("RSA 解密失败: %w", err)
	}
	return plain, nil
}
