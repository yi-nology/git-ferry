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
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	// Match 任务/仓库匹配条件(与 health.Filter 同语义)
	Match map[string][]string `json:"match,omitempty"`
	// Spec 套用到任务的默认值(cron/分支/启用)
	Spec Spec `json:"spec"`
	// Tags 便于检索
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Spec 策略默认值。
type Spec struct {
	Cron           string `json:"cron,omitempty"`
	SourceBranch   string `json:"source_branch,omitempty"`
	TargetBranch   string `json:"target_branch,omitempty"`
	Enabled        *bool  `json:"enabled,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
	// RetryMax 失败自动重跑上限(0=沿用全局)
	RetryMax int `json:"retry_max,omitempty"`
}

// Store 文件型模板库。
type Store struct {
	mu   sync.RWMutex
	path string
	list []Template
}

var ErrNotFound = errors.New("template not found")

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
