// Package config 应用配置（配置文件模式：用户复制 config.example.yaml 后填写自定义密钥）
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration 支持 "120s" 形式的 YAML 时长（yaml.v3 不自带 string→Duration 解析）
type Duration time.Duration

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("解析 duration %q 失败: %w", s, err)
	}
	*d = Duration(dur)
	return nil
}

// ZhihuConfig 知乎开放平台配置
type ZhihuConfig struct {
	AccessSecret    string `yaml:"access_secret"`     // 必填：知乎 Bearer token（明文，与 enc 二选一）
	AccessSecretEnc string `yaml:"access_secret_enc"` // 可选：RSA 公钥加密后的密文（存在时优先，解密覆盖）
	OpenAPIBaseURL  string `yaml:"openapi_base_url"`  // 默认 https://developer.zhihu.com
	SearchURL       string `yaml:"zhihu_search_url"`  // 可选：完整 endpoint，优先级最高
	KnowledgeBaseID string `yaml:"knowledge_base_id"` // 可选：知识库搜索用（股票讨论知识库）
}

// DeepSeekConfig DeepSeek 模型配置
type DeepSeekConfig struct {
	APIKey    string   `yaml:"api_key"`     // 必填（明文，与 enc 二选一）
	APIKeyEnc string   `yaml:"api_key_enc"` // 可选：RSA 公钥加密后的密文（存在时优先）
	BaseURL   string   `yaml:"base_url"`    // 默认 https://api.deepseek.com
	Timeout   Duration `yaml:"timeout"`     // 默认 120s
}

// KeyBoxConfig 密钥箱配置（持久 RSA 密钥对）
type KeyBoxConfig struct {
	PrivateKey string `yaml:"private_key"` // 私钥文件路径（部署者本地，chmod 600）
}

// ServerConfig HTTP 服务配置
type ServerConfig struct {
	Port int `yaml:"port"` // 默认 8080
}

// Config 应用配置
type Config struct {
	Zhihu    ZhihuConfig    `yaml:"zhihu"`
	DeepSeek DeepSeekConfig `yaml:"deepseek"`
	KeyBox   KeyBoxConfig   `yaml:"keybox"`
	Server   ServerConfig   `yaml:"server"`
}

// defaultKeyBoxDir 默认私钥目录：$HOME/.zhihudp
func defaultKeyBoxDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".zhihudp"
	}
	return home + "/.zhihudp"
}

func defaultConfig() *Config {
	c := &Config{}
	c.Zhihu.OpenAPIBaseURL = "https://developer.zhihu.com"
	c.DeepSeek.BaseURL = "https://api.deepseek.com"
	c.DeepSeek.Timeout = Duration(120 * time.Second)
	c.KeyBox.PrivateKey = defaultKeyBoxDir() + "/zhihudp_private.pem"
	c.Server.Port = 8080
	return c
}

// Load 加载配置：环境变量 > config.yaml > 默认值
func Load(path string) (*Config, error) {
	cfg := defaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "[warn] 配置文件 %s 不存在，使用默认值（如需密钥请复制 config.example.yaml 填写）\n", path)
			return cfg, nil
		}
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 环境变量覆盖（便于 CI/部署注入，不设则忽略）
	applyEnv := func(key string, dst *string) {
		if v := os.Getenv(key); v != "" {
			*dst = v
		}
	}
	applyEnv("ZHIHU_ACCESS_SECRET", &cfg.Zhihu.AccessSecret)
	applyEnv("ZHIHU_OPENAPI_BASE_URL", &cfg.Zhihu.OpenAPIBaseURL)
	applyEnv("ZHIHU_ZHIHU_SEARCH_URL", &cfg.Zhihu.SearchURL)
	applyEnv("DEEPSEEK_API_KEY", &cfg.DeepSeek.APIKey)
	applyEnv("DEEPSEEK_BASE_URL", &cfg.DeepSeek.BaseURL)

	// 必填校验：只警告不 panic（调用时再明确报错）
	if cfg.Zhihu.AccessSecret == "" {
		fmt.Fprintln(os.Stderr, "[warn] 未配置 zhihu.access_secret（知乎 Bearer token），搜索接口将不可用")
	}
	if cfg.DeepSeek.APIKey == "" {
		fmt.Fprintln(os.Stderr, "[warn] 未配置 deepseek.api_key，AI 分析将不可用")
	}

	return cfg, nil
}

// String 脱敏：任何日志打印 Config 都不泄露 secret
func (c *Config) String() string {
	mask := func(s string) string {
		if s == "" {
			return "(未配置)"
		}
		if len(s) <= 4 {
			return "****"
		}
		return s[:2] + "****" + s[len(s)-2:]
	}
	keyMode := "明文"
	if c.DeepSeek.APIKeyEnc != "" || c.Zhihu.AccessSecretEnc != "" {
		keyMode = "密文"
	}
	return fmt.Sprintf("Config{zhihu_secret:%s, deepseek_key:%s, key_mode:%s, base_url:%s, port:%d}",
		mask(c.Zhihu.AccessSecret), mask(c.DeepSeek.APIKey), keyMode, c.DeepSeek.BaseURL, c.Server.Port)
}

// PersistEnc 把加密后的密钥密文写回 config.yaml（只写 enc 字段，绝不落明文）。
// 用于开屏「上传密钥」后持久化：重启后加载解密恢复。
func (c *Config) PersistEnc(path, deepseekEnc, zhihuEnc string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			data = []byte{}
		} else {
			return fmt.Errorf("读取配置文件失败: %w", err)
		}
	}
	var raw map[string]any
	if len(data) > 0 {
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("解析配置文件失败: %w", err)
		}
	}
	raw = ensureMap(raw)
	if deepseekEnc != "" {
		setNested(raw, []string{"deepseek", "api_key_enc"}, deepseekEnc)
	}
	if zhihuEnc != "" {
		setNested(raw, []string{"zhihu", "access_secret_enc"}, zhihuEnc)
	}
	out, err := yaml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return fmt.Errorf("写回配置文件失败: %w", err)
	}
	return nil
}

func ensureMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func setNested(m map[string]any, keys []string, val any) {
	cur := m
	for i, k := range keys {
		if i == len(keys)-1 {
			cur[k] = val
			return
		}
		next, ok := cur[k].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[k] = next
		}
		cur = next
	}
}
