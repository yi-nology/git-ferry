// Package tpl 提供同步策略模板库(借鉴 Renovate packageRules/presets)。
// 模板描述「一批仓库该怎么同步」,可反复套用到任务创建/批量更新。
// 存储用 data/templates.json:壳层零 DB 迁移,重启可恢复。
package tpl

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Template 同步策略模板。
type Template struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Extends 继承的基础模板 ID（Renovate preset 模式）。可链式，须无环。
	Extends string `json:"extends,omitempty"`
	// Match 任务/仓库匹配条件(与 health.Filter 同语义)
	Match map[string][]string `json:"match,omitempty"`
	// Spec 套用到任务的默认值(cron/分支/启用)；继承时子覆盖父（非空字段）。
	Spec Spec `json:"spec"`
	// Tags 便于检索
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Spec 策略默认值。非零/非空字段才参与覆盖。
type Spec struct {
	Cron           string `json:"cron,omitempty"`
	SourceBranch   string `json:"source_branch,omitempty"`
	TargetBranch   string `json:"target_branch,omitempty"`
	Enabled        *bool  `json:"enabled,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
	// RetryMax 失败自动重跑上限(0=沿用全局)
	RetryMax int `json:"retry_max,omitempty"`
}

// Merge 将 child 覆盖到 base（child 非空字段优先）。
func (base Spec) Merge(child Spec) Spec {
	out := base
	if child.Cron != "" {
		out.Cron = child.Cron
	}
	if child.SourceBranch != "" {
		out.SourceBranch = child.SourceBranch
	}
	if child.TargetBranch != "" {
		out.TargetBranch = child.TargetBranch
	}
	if child.Enabled != nil {
		e := *child.Enabled
		out.Enabled = &e
	}
	if child.TimeoutSeconds > 0 {
		out.TimeoutSeconds = child.TimeoutSeconds
	}
	if child.RetryMax > 0 {
		out.RetryMax = child.RetryMax
	}
	return out
}

// Store 文件型模板库。
type Store struct {
	mu   sync.RWMutex
	path string
	list []Template
}

var ErrNotFound = errors.New("template not found")

// ErrCycle 继承链成环。
var ErrCycle = errors.New("template extends cycle detected")

// Resolve 沿 Extends 链合并 Spec：祖先在前，子覆盖父。
// 返回 effective Spec 与链路 ID（含自身）。成环返回 ErrCycle。
func (s *Store) Resolve(id string) (Spec, []string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return resolveLocked(s.list, id)
}

// ResolveSelf 不依赖 Store 的纯解析（测试用）。
func ResolveSelf(list []Template, id string) (Spec, []string, error) {
	return resolveLocked(list, id)
}

func resolveLocked(list []Template, id string) (Spec, []string, error) {
	byID := make(map[string]Template, len(list))
	for i := range list {
		byID[list[i].ID] = list[i]
	}
	// 收集链路：自身 → 父 → 祖父…
	chain := []string{}
	seen := map[string]bool{}
	cur := id
	for cur != "" {
		if seen[cur] {
			return Spec{}, append(chain, cur), ErrCycle
		}
		t, ok := byID[cur]
		if !ok {
			if len(chain) == 0 {
				return Spec{}, nil, ErrNotFound
			}
			// 中间父缺失：停止继承，用已合并的
			break
		}
		seen[cur] = true
		chain = append(chain, cur)
		cur = t.Extends
	}
	// 从祖先往子合并
	eff := Spec{}
	for i := len(chain) - 1; i >= 0; i-- {
		eff = eff.Merge(byID[chain[i]].Spec)
	}
	return eff, chain, nil
}

// Effective 返回合并后的模板副本（Spec 已解析，Extends 保留便于展示）。
func (s *Store) Effective(id string) (*Template, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var base *Template
	for i := range s.list {
		if s.list[i].ID == id {
			t := s.list[i]
			base = &t
			break
		}
	}
	if base == nil {
		return nil, ErrNotFound
	}
	spec, chain, err := resolveLocked(s.list, id)
	if err != nil {
		return nil, err
	}
	out := *base
	out.Spec = spec
	// Description 标注继承链，便于 API 消费者理解
	if len(chain) > 1 {
		if out.Description != "" {
			out.Description += " "
		}
		out.Description += "[extends: " + joinIDs(chain[1:]) + "]"
	}
	return &out, nil
}

func joinIDs(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	out := ids[0]
	for _, x := range ids[1:] {
		out += " → " + x
	}
	return out
}

// Open 打开(不存在则空库)。
func Open(path string) (*Store, error) {
	s := &Store{path: path, list: []Template{}}
	data, err := os.ReadFile(path) //nolint:gosec // 配置目录由部署方控制
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(data, &s.list); err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	return s, nil
}

// List 返回全部模板(副本)。
func (s *Store) List() []Template {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Template, len(s.list))
	copy(out, s.list)
	return out
}

// ListByTag 返回带指定标签的模板。
func (s *Store) ListByTag(tag string) []Template {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Template
	for i := range s.list {
		for _, x := range s.list[i].Tags {
			if x == tag {
				out = append(out, s.list[i])
				break
			}
		}
	}
	return out
}

// Get 按 ID 取模板。
func (s *Store) Get(id string) (*Template, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.list {
		if s.list[i].ID == id {
			t := s.list[i]
			return &t, nil
		}
	}
	return nil, ErrNotFound
}

// Upsert 新建或覆盖。
func (s *Store) Upsert(t *Template) (*Template, error) {
	if t.ID == "" {
		t.ID = fmt.Sprintf("tpl-%d", time.Now().UnixNano())
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	found := false
	for i := range s.list {
		if s.list[i].ID != t.ID {
			continue
		}
		t.CreatedAt = s.list[i].CreatedAt
		t.UpdatedAt = now
		s.list[i] = *t
		found = true
		break
	}
	if !found {
		t.CreatedAt, t.UpdatedAt = now, now
		s.list = append(s.list, *t)
	}
	if err := s.persistLocked(); err != nil {
		return nil, err
	}
	out := *t
	return &out, nil
}

// Delete 删除。
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.list {
		if s.list[i].ID == id {
			s.list = append(s.list[:i], s.list[i+1:]...)
			return s.persistLocked()
		}
	}
	return ErrNotFound
}

func (s *Store) persistLocked() error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}
