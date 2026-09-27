package agent

import (
	"github.com/yi-nology/git-ferry/internal/agent/memory"
	"github.com/yi-nology/git-ferry/internal/agent/tools"
)

// memBridge 把 memory.Store 适配到 tools.MemStore。
type memBridge struct{ s *memory.Store }

func (b memBridge) Remember(kind, content string, tags []string) (string, error) {
	e, err := b.s.Remember(memory.Kind(kind), content, tags)
	if err != nil {
		return "", err
	}
	return e.ID, nil
}

func (b memBridge) Recall(query string, limit int) []tools.MemEntry {
	items := b.s.Recall(query, limit)
	out := make([]tools.MemEntry, 0, len(items))
	for i := range items {
		out = append(out, tools.MemEntry{
			ID: items[i].ID, Kind: string(items[i].Kind), Content: items[i].Content, Tags: items[i].Tags,
		})
	}
	return out
}

func (b memBridge) Forget(id string) bool { return b.s.Forget(id) }

// NewMemBridge 构造适配器。
func NewMemBridge(s *memory.Store) tools.MemStore {
	if s == nil {
		return nil
	}
	return memBridge{s: s}
}

// Manifest 记忆摘要(注入 system prompt)。
func MemoryManifest(s *memory.Store, limit int) string {
	if s == nil {
		return ""
	}
	return s.Manifest(limit)
}
