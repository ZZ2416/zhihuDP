// 入口：加载配置 → 组装依赖与 HTTP 层 → 启动服务 / CLI 模式
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"zhihudp/internal/agent"
	"zhihudp/internal/chat"
	"zhihudp/internal/config"
	"zhihudp/internal/finance"
	"zhihudp/internal/hot"
	"zhihudp/internal/keybox"
	"zhihudp/internal/kline"
	"zhihudp/internal/minute"
	"zhihudp/internal/news"
	"zhihudp/internal/server"
	"zhihudp/internal/stock"
	"zhihudp/internal/types"
	"zhihudp/internal/video"
	"zhihudp/web"
)

func main() {
	configPath := flag.String("config", "config.yaml", "配置文件路径（默认 config.yaml）")
	query := flag.String("q", "", "CLI 模式：运行一次分析并打印事件（如 -q 茅台）")
	cmdKeygen := flag.Bool("keygen", false, "生成持久 RSA 密钥对并加密你的密钥（输出密文，可写入 config.yaml）")
	cmdEnc := flag.String("enc", "", "用公钥加密一个密钥，输出 base64 密文（如 -enc sk-xxx）")
	cmdPubkey := flag.Bool("pubkey", false, "打印持久公钥 PEM（手动加密用）")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 密钥工具子命令（不启动服务）
	if *cmdKeygen || *cmdPubkey || *cmdEnc != "" {
		runKeyTool(cfg, *cmdKeygen, *cmdPubkey, *cmdEnc, *configPath)
		return
	}

	// 加载持久密钥对（私钥仅部署者本地，chmod 600）
	kb, err := keybox.New(cfg.KeyBox.PrivateKey)
	if err != nil {
		log.Fatalf("初始化密钥箱失败: %v", err)
	}
	// 解密密文密钥（config.yaml 的 *_enc 字段）→ 覆盖明文，代码/配置无明文
	if err := decryptEncKeys(cfg, kb); err != nil {
		log.Fatalf("解密密钥失败: %v", err)
	}
	log.Printf("配置加载完成: %s", cfg.String())

	// CLI 模式（快速验证用，不启动 HTTP）
	if *query != "" {
		runCLI(*query, cfg)
		return
	}

	// 组装依赖 + HTTP 层（依赖注入：各包无全局状态）
	ks := &keyService{KeyBox: kb, cfg: cfg, configPath: *configPath}
	deps := buildDeps(cfg)

	// 二期：看山追问对话服务（会话按股票隔离，快照由 /api/ask 捕获）
	chatSvc := chat.New(chat.NewStore(10), chat.Deps{
		Quote: func(ctx context.Context, market, code string) (*types.Quote, error) {
			k, err := kline.GetKline(ctx, market, code, 1) // 仅取报价段，不取 K 线序列
			if err != nil {
				return nil, err
			}
			return &k.Quote, nil
		},
		Finance: func(ctx context.Context, code, market string) (*types.FinanceResult, error) {
			return finance.GetResult(ctx, code, market)
		},
		ChatAgent: func(ctx context.Context, facts types.ChatFacts, history []types.ChatMessage, message string, sink func(types.Event) error) error {
			return agent.Chat(ctx, facts, history, message, deps, sink)
		},
	})

	// 财报解析服务（东财双源 + AI 解析）
	fp := &financeProvider{
		get: func(ctx context.Context, code, market string) (*types.FinanceResult, error) {
			return finance.GetResult(ctx, code, market)
		},
		analyze: func(ctx context.Context, code, market string, sink func(types.Event) error) error {
			res, err := finance.GetResult(ctx, code, market)
			if err != nil {
				return err
			}
			return agent.AnalyzeFinance(ctx, code, res.Name, res.Indicators, deps, sink)
		},
	}

	// 分时数据（东财主 + 腾讯兜底）
	mp := &minuteProvider{get: minute.GetMinute}
	// 视频资讯（B站）
	vp := &videoProvider{get: video.GetVideos}

	srv := server.New(
		analyzerFunc(func(ctx context.Context, stock string, sink func(types.Event) error) error {
			return agent.RunAnalysis(ctx, stock, deps, sink)
		}),
		resolverFunc(stock.Resolve),
		klineProviderFunc(kline.GetKline),
		newsProviderFunc(news.GetNews),
		hotProviderFunc{getHot: hot.GetHot, getSectorStocks: hot.GetSectorStocks},
		ks,              // 密钥箱：公钥下发 + 加密密钥热更新
		chatSvc,         // 二期：看山追问对话
		fp,              // 财报解析（东财双源 + AI）
		mp,              // 分时数据（东财主 + 腾讯兜底）
		vp,              // 视频资讯（B站）
		cfg.Media.Dir,   // 媒体目录（/media/ 播放；空 = 禁用）
		cfg.Media.Token, // 媒体访问令牌（抖音式禁止转载：无/错 token 403）
		web.FS,          // 前端资源（go:embed 内嵌）
	)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("zhihuDP 启动完成，访问 http://localhost%s", addr)
	if err := http.ListenAndServe(addr, srv.Routes()); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// analyzerFunc / resolverFunc 适配器：函数实现 → server 方法型接口
type analyzerFunc func(ctx context.Context, stock string, sink func(types.Event) error) error

func (f analyzerFunc) RunAnalysis(ctx context.Context, stock string, sink func(types.Event) error) error {
	return f(ctx, stock, sink)
}

type resolverFunc func(ctx context.Context, q string) (*types.StockInfo, error)

func (f resolverFunc) Resolve(ctx context.Context, q string) (*types.StockInfo, error) {
	return f(ctx, q)
}

// 编译期断言：适配器满足 server 接口（house style）
var (
	_ server.Analyzer      = (analyzerFunc)(nil)
	_ server.Resolver      = (resolverFunc)(nil)
	_ server.KlineProvider = (klineProviderFunc)(nil)
	_ server.NewsProvider  = (newsProviderFunc)(nil)
	_ server.HotProvider   = hotProviderFunc{}
)

// klineProviderFunc 适配器：函数实现 → server.KlineProvider 接口
type klineProviderFunc func(ctx context.Context, market, code string, days int) (*types.Kline, error)

func (f klineProviderFunc) GetKline(ctx context.Context, market, code string, days int) (*types.Kline, error) {
	return f(ctx, market, code, days)
}

// newsProviderFunc 适配器：函数实现 → server.NewsProvider 接口
type newsProviderFunc func(ctx context.Context, keyword string, count int) ([]types.NewsItem, error)

func (f newsProviderFunc) GetNews(ctx context.Context, keyword string, count int) ([]types.NewsItem, error) {
	return f(ctx, keyword, count)
}

// hotProviderFunc 适配器：函数实现 → server.HotProvider 接口
type hotProviderFunc struct {
	getHot          func(ctx context.Context, typ string, count int) ([]types.HotItem, error)
	getSectorStocks func(ctx context.Context, code string, count int) ([]types.HotItem, error)
}

func (f hotProviderFunc) GetHot(ctx context.Context, typ string, count int) ([]types.HotItem, error) {
	return f.getHot(ctx, typ, count)
}

func (f hotProviderFunc) GetSectorStocks(ctx context.Context, code string, count int) ([]types.HotItem, error) {
	return f.getSectorStocks(ctx, code, count)
}

// keyService 密钥箱服务：RSA 加解密 + 热更新运行中的配置 + 密文持久化（实现 server.KeyService）
type keyService struct {
	*keybox.KeyBox
	cfg        *config.Config
	configPath string // config.yaml 路径（上传密钥后持久化密文用）
}

// UpdateKeys 应用用户提交的 DeepSeek 密钥（空值保留原密钥）
func (k *keyService) UpdateKeys(deepseekKey string) error {
	if deepseekKey != "" {
		k.cfg.DeepSeek.APIKey = deepseekKey
	}
	return nil
}

// PersistKeys 把加密后的密钥密文写回 config.yaml（只写 *_enc 字段，绝不落明文），
// 重启后加载解密恢复 —— 仓库/配置泄露也只是密文。
func (k *keyService) PersistKeys(deepseekKeyEnc string) error {
	return k.cfg.PersistEnc(k.configPath, deepseekKeyEnc)
}

// financeProvider 财报服务适配器：数据（finance dao）+ AI 解析（agent）
type financeProvider struct {
	get     func(ctx context.Context, code, market string) (*types.FinanceResult, error)
	analyze func(ctx context.Context, code, market string, sink func(types.Event) error) error
}

func (f *financeProvider) GetFinance(ctx context.Context, code, market string) (*types.FinanceResult, error) {
	return f.get(ctx, code, market)
}

func (f *financeProvider) AnalyzeFinance(ctx context.Context, code, market string, sink func(types.Event) error) error {
	return f.analyze(ctx, code, market, sink)
}

// minuteProvider 分时适配器
type minuteProvider struct {
	get func(ctx context.Context, market, code string) (*types.MinuteResult, error)
}

func (m *minuteProvider) GetMinute(ctx context.Context, market, code string) (*types.MinuteResult, error) {
	return m.get(ctx, market, code)
}

// videoProvider 视频资讯适配器
type videoProvider struct {
	get func(ctx context.Context, keyword string, count int) ([]types.VideoItem, error)
}

func (v *videoProvider) GetVideos(ctx context.Context, keyword string, count int) ([]types.VideoItem, error) {
	return v.get(ctx, keyword, count)
}

// 编译期断言：videoProvider 满足 server.VideoProvider
var _ server.VideoProvider = (*videoProvider)(nil)

// 编译期断言：minuteProvider 满足 server.MinuteProvider
var _ server.MinuteProvider = (*minuteProvider)(nil)

// 编译期断言：financeProvider 满足 server.FinanceProvider
var _ server.FinanceProvider = (*financeProvider)(nil)

// 编译期断言：keyService 满足 server.KeyService
var _ server.KeyService = (*keyService)(nil)

// decryptEncKeys 用持久私钥解密密文密钥（config.yaml 的 *_enc 字段）并覆盖明文
func decryptEncKeys(cfg *config.Config, kb *keybox.KeyBox) error {
	if cfg.DeepSeek.APIKeyEnc != "" {
		plain, err := kb.DecryptOAEPBase64(cfg.DeepSeek.APIKeyEnc)
		if err != nil {
			return fmt.Errorf("解密 deepseek.api_key_enc 失败: %w", err)
		}
		cfg.DeepSeek.APIKey = string(plain)
	}
	return nil
}

// runKeyTool 密钥工具子命令：keygen（生成密钥对 + 交互加密）/ pubkey / enc
func runKeyTool(cfg *config.Config, doKeygen, doPubkey bool, encValue, configPath string) {
	kb, err := keybox.New(cfg.KeyBox.PrivateKey)
	if err != nil {
		log.Fatalf("初始化密钥箱失败: %v", err)
	}
	if doPubkey {
		fmt.Print(kb.PublicKeyPEM())
		return
	}
	if encValue != "" {
		ct, err := keybox.EncryptOAEPBase64(kb.PublicKeyPEM(), encValue)
		if err != nil {
			log.Fatalf("加密失败: %v", err)
		}
		fmt.Println(ct)
		return
	}
	if doKeygen {
		fmt.Printf("密钥对就绪：私钥 %s（600 权限）\n\n", cfg.KeyBox.PrivateKey)
		fmt.Println("输入你的真实密钥（终端不回显），用公钥加密生成密文：")
		reader := bufio.NewReader(os.Stdin)
		readLine := func(prompt string) string {
			fmt.Print(prompt)
			s, _ := reader.ReadString('\n')
			return strings.TrimSpace(s)
		}
		ds := readLine("DeepSeek API Key: ")
		zh := readLine("知乎 Access Secret: ")
		dsEnc, _ := keybox.EncryptOAEPBase64(kb.PublicKeyPEM(), ds)
		zhEnc, _ := keybox.EncryptOAEPBase64(kb.PublicKeyPEM(), zh)
		fmt.Println("\n=== 请把以下两行粘贴进 config.yaml（替换 deepseek 与 zhihu 下的对应字段）===")
		if dsEnc != "" {
			fmt.Printf("deepseek:\n  api_key_enc: %q\n", dsEnc)
		}
		if zhEnc != "" {
			fmt.Printf("zhihu:\n  access_secret_enc: %q\n", zhEnc)
		}
		fmt.Println("\n（config.yaml 也可留空 *_enc 字段，改用开屏弹窗上传）")
	}
}

// buildDeps 组装 agent 依赖（业务层接线点）
func buildDeps(cfg *config.Config) agent.Deps {
	return agent.Deps{
		ResolveStock: stock.Resolve,
		// getter：每次调用读取最新配置（支持弹窗热更新密钥后立即生效）
		DeepSeek: func() config.DeepSeekConfig { return cfg.DeepSeek },
	}
}

// runCLI 命令行模式：跑一次分析，打印事件
func runCLI(query string, cfg *config.Config) {
	deps := buildDeps(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	err := agent.RunAnalysis(ctx, query, deps, func(ev types.Event) error {
		b, _ := json.Marshal(ev.Data)
		fmt.Printf("[%s] %s\n", ev.Type, b)
		return nil
	})
	if err != nil {
		fmt.Printf("[error] %v\n", err)
	}
}
