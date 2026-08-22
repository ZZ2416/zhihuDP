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

// Generate 完整流程：resolve 股票 → AI 生成 → 厂商校验过滤
func (s *Service) Generate(ctx context.Context, code, market string) (*types.ChainResult, error) {
	info, err := s.deps.Resolve(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("股票识别失败: %w", err)
	}
	res, err := s.deps.Generate(ctx, info.Name, info.Code)
	if err != nil {
		return nil, err
	}
	// 厂商代码校验（AI 可能编造代码；受控并发 5，避免串行慢且防东财限流）
	const maxConcurrent = 5
	sem := make(chan struct{}, maxConcurrent)
	var (
		mu      sync.Mutex
		removed int
		wg      sync.WaitGroup
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
			bad := 0
			for _, c := range list {
				found, err := s.deps.Resolve(ctx, c.Code)
				if err != nil || found == nil {
					bad++
					continue
				}
				valid = append(valid, types.ChainCompany{Code: found.Code, Name: found.Name})
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
	if removed > 0 {
		res.Degraded = true
		res.ErrMsg = fmt.Sprintf("AI 生成仅供参考：%d 个厂商代码未通过校验已过滤", removed)
	}
	return res, nil
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
