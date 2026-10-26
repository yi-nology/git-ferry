package acp

import (
	"sync"

	"github.com/yi-nology/git-ferry/internal/agent/engine"
)

// fakeSession 是 engine.Session 的测试替身。
//
// 真实实现是 internal/agent.Session(要连 SessionStore),
// 这里只要满足契约即可,不去依赖 agent 包(避免包间成环)。
type fakeSession struct {
	id       string
	mu       sync.Mutex
	messages []engine.Turn
}

func newFakeSession(id string) *fakeSession { return &fakeSession{id: id} }

func (s *fakeSession) SessionID() string { return s.id }

func (s *fakeSession) History() []engine.Turn {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]engine.Turn, len(s.messages))
	copy(out, s.messages)
	return out
}

func (s *fakeSession) Append(role, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, engine.Turn{Role: role, Content: content})
}

// Text 拼出助手回复文本(测试断言用)。
func (s *fakeSession) Text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out string
	for _, m := range s.messages {
		if m.Role == "assistant" {
			out += m.Content
		}
	}
	return out
}

// —— tools.SessionScope ——

func (s *fakeSession) SetPending(string, string) (string, error) { return "", nil }
func (s *fakeSession) ConsumeConsumed(string, string) bool       { return false }
func (s *fakeSession) MarkConsumed(string, string)               {}
func (s *fakeSession) SetLastConfirm(string, string, string)     {}
func (s *fakeSession) LastConfirm() (string, string, string)     { return "", "", "" }
func (s *fakeSession) SetPermissionMode(string) bool             { return true }
func (s *fakeSession) PermissionMode() string                    { return "" }

func (s *fakeSession) ConsumePending(string, string) (string, error) { return "", nil }
