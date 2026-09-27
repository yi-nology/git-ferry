// Package memory 提供 AI 助手的持久记忆(借鉴 zcode memory extraction/recall)。
//
// 目标:让助手跨会话记住「哪类失败怎么处理」「用户偏好」「高频任务」,
// 而不是每次都从零推理。存储为 data/ai-memory.json,结构简单可审计。
package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Kind 记忆类型。
type Kind string

const (
	KindPreference Kind = "preference" // 用户偏好/约定
	KindPattern    Kind = "pattern"    // 故障模式与处置
	KindFact       Kind = "fact"       // 环境/仓库事实
	KindTask       Kind = "task"       // 待办/约定动作
)

// Entry 一条记忆。
type Entry struct {
	ID        string    `json:"id"`
	Kind      Kind      `json:"kind"`
	Content   string    `json:"content"`
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	UseCount  int       `json:"use_count"`
}

// Store 文件型记忆库(并发安全)。
type Store struct {
	mu      sync.RWMutex
	path    string
	entries []Entry
}

// Open 打开(不存在则空库)。
func Open(path string) (*Store, error) {
	s := &Store{path: path}
	data, err := os.ReadFile(path) //nolint:gosec // 数据目录由部署方控制
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &s.entries); err != nil {
			return nil, fmt.Errorf("parse ai memory: %w", err)
		}
	}
	return s, nil
}

// Remember 写入一条记忆;content 为空不写。同 Kind+Content 视为更新。
func (s *Store) Remember(kind Kind, content string, tags []string) (*Entry, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("empty memory content")
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.entries {
		if s.entries[i].Kind != kind || s.entries[i].Content != content {
			continue
		}
		s.entries[i].UpdatedAt = now
		s.entries[i].Tags = mergeTags(s.entries[i].Tags, tags)
		s.entries[i].UseCount++
		if err := s.persistLocked(); err != nil {
			return nil, err
		}
		out := s.entries[i]
		return &out, nil
	}
	e := Entry{
		ID:        fmt.Sprintf("mem-%d", now.UnixNano()),
		Kind:      kind,
		Content:   content,
		Tags:      tags,
		CreatedAt: now,
		UpdatedAt: now,
		UseCount:  1,
	}
	s.entries = append(s.entries, e)
	// 上限 500 条,按更新时间淘汰最旧
	if len(s.entries) > 500 {
		sort.Slice(s.entries, func(i, j int) bool { return s.entries[i].UpdatedAt.After(s.entries[j].UpdatedAt) })
		s.entries = s.entries[:500]
	}
	if err := s.persistLocked(); err != nil {
		return nil, err
	}
	return &e, nil
}

// Recall 按关键字/标签召回,最多 limit 条。
// score: 命中标签>命中内容>全部;同分按更新时间倒序。
func (s *Store) Recall(query string, limit int) []Entry {
	if limit <= 0 {
		limit = 8
	}
	q := strings.ToLower(strings.TrimSpace(query))
	s.mu.RLock()
	defer s.mu.RUnlock()
	type scored struct {
		e Entry
		n int
	}
	var hits []scored
	for i := range s.entries {
		e := &s.entries[i]
		n := 0
		if q == "" {
			n = 1
		} else {
			if strings.Contains(strings.ToLower(e.Content), q) {
				n += 2
			}
			for _, t := range e.Tags {
				if strings.Contains(strings.ToLower(t), q) {
					n += 3
				}
			}
			if strings.Contains(strings.ToLower(string(e.Kind)), q) {
				n++
			}
		}
		if n > 0 {
			hits = append(hits, scored{*e, n})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].n != hits[j].n {
			return hits[i].n > hits[j].n
		}
		return hits[i].e.UpdatedAt.After(hits[j].e.UpdatedAt)
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]Entry, 0, len(hits))
	for i := range hits {
		out = append(out, hits[i].e)
	}
	return out
}

// Manifest 供注入 system prompt 的摘要。
func (s *Store) Manifest(limit int) string {
	items := s.Recall("", limit)
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## 已记住的偏好与经验\n")
	for i := range items {
		fmt.Fprintf(&b, "- [%s] %s\n", items[i].Kind, items[i].Content)
	}
	return b.String()
}

// Forget 按 ID 删除。
func (s *Store) Forget(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.entries {
		if s.entries[i].ID == id {
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			_ = s.persistLocked()
			return true
		}
	}
	return false
}

func (s *Store) persistLocked() error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

func mergeTags(a, b []string) []string {
	set := map[string]bool{}
	for _, t := range append(a, b...) {
		if t = strings.TrimSpace(t); t != "" {
			set[t] = true
		}
	}
	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}
