package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPersistEncOnlyWritesEncFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	orig := "zhihu:\n  access_secret: \"plain-secret\"\n  openapi_base_url: \"https://developer.zhihu.com\"\ndeepseek:\n  api_key: \"sk-plain\"\nserver:\n  port: 8080\n"
	if err := os.WriteFile(path, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.PersistEnc(path, "ENC_DS", "ENC_ZH"); err != nil {
		t.Fatalf("PersistEnc 失败: %v", err)
	}
	// 重新加载：明文保留，enc 新增
	cfg2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.DeepSeek.APIKey != "sk-plain" || cfg2.Zhihu.AccessSecret != "plain-secret" {
		t.Errorf("明文被覆盖: %+v", cfg2)
	}
	if cfg2.DeepSeek.APIKeyEnc != "ENC_DS" || cfg2.Zhihu.AccessSecretEnc != "ENC_ZH" {
		t.Errorf("enc 字段未写入: deepseek=%q zhihu=%q", cfg2.DeepSeek.APIKeyEnc, cfg2.Zhihu.AccessSecretEnc)
	}
	// 文件中不应出现明文密钥（检查写入内容不含明文？明文本来就有，只验证 enc 存在即可）
	data, _ := os.ReadFile(path)
	if !contains(string(data), "api_key_enc: ENC_DS") || !contains(string(data), "access_secret_enc: ENC_ZH") {
		t.Errorf("写回文件缺少 enc 字段:\n%s", data)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(len(s) > 0 && (indexOf(s, sub) >= 0)))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
