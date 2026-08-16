// Package keybox 密钥箱：持久 RSA 公私钥加解密。
// 用途：部署者本地生成密钥对——公钥加密真实 API 密钥得到密文（可安全入库/入仓库），
// 私钥文件仅存部署者本地（chmod 600）；服务器启动加载私钥解密使用，明文不出进程。
// 前端弹窗上传的密钥同样用该公钥加密传输，服务器私钥解密后热更新并持久化密文。
package keybox

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// KeyBox 持有一对 RSA 密钥：公钥下发给前端加密，私钥仅服务端持有。
// 与 v1 的区别：密钥对**持久化**（私钥文件落本地），重启后公钥不变，
// 从而允许把密文密钥入库（v1 随机密钥对无法入库，重启即失效）。
type KeyBox struct {
	mu   sync.RWMutex
	priv *rsa.PrivateKey
	pub  string // PEM 公钥，缓存供 GET /api/config/pubkey
}

// New 加载或生成持久 RSA 密钥对：
//   - 私钥文件已存在 → 加载（兼容 PKCS1 / PKCS8）
//   - 不存在 → 生成 2048 位密钥对并写入（目录自建，文件 chmod 600）
func New(privateKeyPath string) (*KeyBox, error) {
	if privateKeyPath == "" {
		return nil, fmt.Errorf("私钥路径不能为空（config.keybox.private_key）")
	}
	if data, err := os.ReadFile(privateKeyPath); err == nil {
		block, _ := pem.Decode(data)
		if block == nil {
			return nil, fmt.Errorf("私钥文件 %s 不是 PEM 格式", privateKeyPath)
		}
		priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			k, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
			if err2 != nil {
				return nil, fmt.Errorf("解析私钥失败: %v", err)
			}
			var ok bool
			priv, ok = k.(*rsa.PrivateKey)
			if !ok {
				return nil, fmt.Errorf("私钥不是 RSA")
			}
		}
		return fromPrivate(priv)
	}
	// 生成并持久化
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("生成 RSA 密钥对失败: %w", err)
	}
	if err := SavePrivateKey(privateKeyPath, priv); err != nil {
		return nil, err
	}
	kb, err := fromPrivate(priv)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(os.Stderr, "[keybox] 已生成持久密钥对，私钥保存于 %s（请妥善保管，勿入库/勿分享）\n", privateKeyPath)
	return kb, nil
}

// fromPrivate 由私钥构建 KeyBox（派生 PEM 公钥）
func fromPrivate(priv *rsa.PrivateKey) (*KeyBox, error) {
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("编码公钥失败: %w", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	return &KeyBox{priv: priv, pub: string(pubPEM)}, nil
}

// SavePrivateKey 私钥写入本地文件（目录自建，chmod 600）
func SavePrivateKey(path string, priv *rsa.PrivateKey) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("创建私钥目录失败: %w", err)
	}
	data := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("写入私钥文件失败: %w", err)
	}
	return os.Chmod(path, 0o600)
}

// PublicKeyPEM 返回公钥 PEM（可安全下发给前端 / 用于手动加密）
func (k *KeyBox) PublicKeyPEM() string {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.pub
}

// DecryptOAEPBase64 用私钥解密 base64(RSA-OAEP 密文)；空串输入返回空（表示未提供，跳过）
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

// EncryptOAEPBase64 用公钥 PEM 加密明文 → base64 密文（CLI 加密密钥用，与前端 Web Crypto 同算法）
func EncryptOAEPBase64(pubPEM, plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	block, _ := pem.Decode([]byte(pubPEM))
	if block == nil {
		return "", fmt.Errorf("公钥 PEM 解析失败")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("解析公钥失败: %w", err)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return "", fmt.Errorf("公钥不是 RSA")
	}
	ct, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, rsaPub, []byte(plain), nil)
	if err != nil {
		return "", fmt.Errorf("RSA 加密失败: %w", err)
	}
	return base64.StdEncoding.EncodeToString(ct), nil
}
