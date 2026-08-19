// Package watchlist service 层：自选池（上限 20 只，本地 JSON 文件持久化）
package watchlist

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"zhihudp/internal/types"
)

// Store 自选池存储（线程安全，文件持久化）
type Store struct {
	mu    sync.Mutex
	items []types.WatchItem
	max   int
	path  string // JSON 文件路径
}

// New 创建/加载自选池（max<=0 用默认 20）
func New(path string, max int) *Store {
	if max <= 0 {
		max = 20
	}
	s := &Store{items: []types.WatchItem{}, max: max, path: path}
	_ = s.load() // 加载失败（首次无文件）静默
	return s
}

// List 返回条目副本（空列表返回 [] 而非 null）
func (s *Store) List() []types.WatchItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]types.WatchItem, len(s.items))
	copy(out, s.items)
	return out
}

// Add 添加（去重 + 上限）；返回剩余可加数
func (s *Store) Add(code, market string) (int, error) {
	code = trim(code)
	if code == "" {
		return 0, fmt.Errorf("股票代码不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, it := range s.items {
		if it.Code == code {
			return s.max - len(s.items), nil // 已存在，幂等
		}
	}
	if len(s.items) >= s.max {
		return 0, fmt.Errorf("自选池已满（上限 %d 只），请先移除", s.max)
	}
	s.items = append(s.items, types.WatchItem{Code: code, Market: market})
	if err := s.save(); err != nil {
		// 保存失败回滚内存
		s.items = s.items[:len(s.items)-1]
		return 0, fmt.Errorf("自选池保存失败: %w", err)
	}
	return s.max - len(s.items), nil
}

// Remove 移除
func (s *Store) Remove(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, it := range s.items {
		if it.Code == trim(code) {
			s.items = append(s.items[:i], s.items[i+1:]...)
			return s.save()
		}
	}
	return nil // 不存在视为成功（幂等）
}

// load 从文件加载
func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var items []types.WatchItem
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	if len(items) > s.max {
		items = items[:s.max]
	}
	s.items = items
	return nil
}

// save 写入文件（目录自建）
func (s *Store) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

func trim(s string) string {
	return strings.TrimSpace(s)
}
