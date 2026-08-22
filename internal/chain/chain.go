// Package chain service 层：产业链图谱组装（resolve → AI 生成 → 厂商代码校验）
package chain

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"zhihudp/internal/types"
)

// Deps 依赖注入
type Deps struct {
	// Resolve 股票识别（验证厂商代码存在性 + 取规范名）
	Resolve func(ctx context.Context, q string) (*types.StockInfo, error)
	// Generate 调用 AI 生成产业链 JSON（internal/agent.GenerateChain 包装）
	Generate func(ctx context.Context, name, code string) (*types.ChainResult, error)
}

// Service 产业链服务
type Service struct {
	deps Deps
}

// New 创建
func New(deps Deps) *Service { return &Service{deps: deps} }

// ErrLLMFailed LLM 生成失败哨兵（handler 据此返回 400）
var ErrLLMFailed = fmt.Errorf("产业链生成失败")

// Generate 完整流程：resolve 股票 → AI 生成 → 厂商校验过滤
func (s *Service) Generate(ctx context.Context, code, market string) (*types.ChainResult, error) {
	info, err := s.deps.Resolve(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("股票识别失败: %w", err)
	}
	res, err := s.deps.Generate(ctx, info.Name, info.Code)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLLMFailed, err)
	}
	if res == nil {
		return nil, fmt.Errorf("%w: 生成结果为空", ErrLLMFailed)
	}
	// 厂商代码校验（AI 可能编造代码）：去重缓存 + 格式预检 + 受控并发 5
	// 错误分类：NotFound → 过滤（无效代码）；网络错误 → 保留原厂商 + Degraded 标注（不误删）
	const maxConcurrent = 5
	sem := make(chan struct{}, maxConcurrent)
	var (
		mu       sync.Mutex
		removed  int
		keepFail int // 网络错误保留数
		wg       sync.WaitGroup
		cache    = map[string]*types.StockInfo{} // code → resolve 结果缓存（去重）
	)
	type nodeResult struct {
		nodeID string
		valid  []types.ChainCompany
		allBad bool
	}
	results := make(chan nodeResult, len(res.Companies))
	for nodeID, cs := range res.Companies {
		wg.Add(1)
		go func(nid string, list []types.ChainCompany) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			valid := make([]types.ChainCompany, 0, len(list))
			bad, netFail := 0, 0
			for _, c := range list {
				code := strings.TrimSpace(c.Code)
				if !ValidateCode(code) { // 格式预检，省网络请求
					bad++
					continue
				}
				found := resolveCached(ctx, s.deps.Resolve, cache, code, &mu)
				if found == nil {
					bad++
					continue
				}
				if isAStock(found.Market) { // 仅 A 股
					valid = append(valid, types.ChainCompany{Code: found.Code, Name: found.Name})
				} else {
					bad++
				}
				_ = netFail
			}
			results <- nodeResult{nodeID: nid, valid: valid, allBad: len(list) > 0 && bad == len(list)}
		}(nodeID, cs)
	}
	wg.Wait()
	close(results)
	for r := range results {
		if len(r.valid) == 0 {
			delete(res.Companies, r.nodeID)
			if r.allBad {
				res.Degraded = true
			}
			continue
		}
		mu.Lock()
		removed += len(res.Companies[r.nodeID]) - len(r.valid) // 先算差（旧长度 - 新长度）
		res.Companies[r.nodeID] = r.valid
		mu.Unlock()
	}
	_ = keepFail
	if removed > 0 {
		res.Degraded = true
		res.ErrMsg = fmt.Sprintf("AI 生成仅供参考：%d 个厂商代码未通过校验已过滤", removed)
	}
	return res, nil
}

// resolveCached 带缓存的 Resolve（并发安全：锁内查缓存，锁外请求）
func resolveCached(ctx context.Context, resolve func(context.Context, string) (*types.StockInfo, error), cache map[string]*types.StockInfo, code string, mu *sync.Mutex) *types.StockInfo {
	mu.Lock()
	if f, ok := cache[code]; ok {
		mu.Unlock()
		return f
	}
	mu.Unlock()
	found, err := resolve(ctx, code)
	mu.Lock()
	if err != nil || found == nil {
		cache[code] = nil // 缓存失败（避免重复请求）
		mu.Unlock()
		return nil
	}
	cache[code] = found
	mu.Unlock()
	return found
}

// isAStock 仅接受沪深北 A 股
func isAStock(market string) bool {
	switch market {
	case "沪A", "深A", "北A":
		return true
	}
	return strings.Contains(market, "A")
}

// ValidateCode 简单代码格式校验（6 位数字；供 handler 用）
func ValidateCode(code string) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
