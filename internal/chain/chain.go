// Package chain service 层：产业链图谱组装（resolve → AI 生成 → 厂商代码校验）
package chain

import (
	"context"
	"fmt"
	"strings"

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
	// 厂商代码校验（AI 可能编造代码）
	removed := 0
	for nodeID, cs := range res.Companies {
		valid := make([]types.ChainCompany, 0, len(cs))
		for _, c := range cs {
			found, err := s.deps.Resolve(ctx, c.Code)
			if err != nil || found == nil {
				removed++
				continue
			}
			valid = append(valid, types.ChainCompany{Code: found.Code, Name: found.Name})
		}
		if len(valid) == 0 {
			delete(res.Companies, nodeID)
			res.Degraded = true
		} else {
			res.Companies[nodeID] = valid
		}
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
