package agent

import (
	"context"
	"errors"
	"github.com/yi-nology/git-ferry/internal/pkg/textutil"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ErrSessionNotFound 会话不存在或已过期。
var ErrSessionNotFound = errors.New("ai_session_not_found")

const (
	sessionTTL       = 30 * time.Minute
	sessionMaxRounds = 20
)

// Message 一轮对话中的一条消息。
type Message struct {
	Role    string // user | assistant
	Content string
}

// Session 一次对话上下文。字段由 SessionStore 管理,外部只读;
// 确认相关方法见 confirm.go(实现 tools.SessionScope 适配,见 runner.go)。
type Session struct {
	ID        string
	Messages  []Message // 已完成轮次(不含进行中输出),超过 maxRounds 淘汰最旧
	CreatedAt time.Time
	LastAt    time.Time

	store *SessionStore

	pending *PendingConfirm // 待确认的危险工具调用(同一时刻至多一个)

	muConfirm    sync.Mutex
	lastConfirm  *PendingConfirm // 最近一次发给前端的确认请求(取即清空)
	consumedTool string          // 确认后直达执行的放行标记
	consumedArgs string
}

// SessionStore 内存会话存储。服务重启即失效(前端收到 404 后重开会话)。
type SessionStore struct {
	mu        sync.Mutex
	sessions  map[string]*Session
	ttl       time.Duration
	maxRounds int
}

func NewSessionStore(ttl time.Duration, maxRounds int) *SessionStore {
	return &SessionStore{
		sessions:  make(map[string]*Session),
		ttl:       ttl,
		maxRounds: maxRounds,
	}
}

func (st *SessionStore) Create() *Session {
	now := time.Now()
	s := &Session{ID: uuid.NewString(), CreatedAt: now, LastAt: now, store: st}
	st.mu.Lock()
	defer st.mu.Unlock()
	st.sessions[s.ID] = s
	return s
}

// Get 取会话;不存在或超过 TTL 返回 ErrSessionNotFound(过期即删)。
func (st *SessionStore) Get(id string) (*Session, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	s, ok := st.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	if time.Since(s.LastAt) > st.ttl {
		delete(st.sessions, id)
		return nil, ErrSessionNotFound
	}
	return s, nil
}

// Append 追加消息并把会话压入活跃态;超过 maxRounds(×2 条)淘汰最旧轮次。
func (st *SessionStore) Append(s *Session, role, content string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	s.Messages = append(s.Messages, Message{Role: role, Content: content})
	maxMsgs := st.maxRounds * 2
	if len(s.Messages) > maxMsgs {
		s.Messages = s.Messages[len(s.Messages)-maxMsgs:]
	}
	s.LastAt = time.Now()
}

// Snapshot 返回会话消息的拷贝:构建模型输入用,避免与并发 Append 竞态。
func (st *SessionStore) Snapshot(s *Session) []Message {
	st.mu.Lock()
	defer st.mu.Unlock()
	out := make([]Message, len(s.Messages))
	copy(out, s.Messages)
	return out
}

// StartJanitor 启动周期性过期回收:Get 只做惰性清理,再无人访问的会话
// (如爬虫创建后即弃)靠 janitor 兜底,防止 map 只增不减。
func (st *SessionStore) StartJanitor(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				st.mu.Lock()
				now := time.Now()
				for id, s := range st.sessions {
					if now.Sub(s.LastAt) > st.ttl {
						delete(st.sessions, id)
					}
				}
				st.mu.Unlock()
			}
		}
	}()
}

// SetPending 在会话上登记一次待确认调用(代理给 Session 的接口适配)。
func (s *Session) SetPending(toolName, argsJSON string) (string, error) {
	if s.store == nil {
		return "", errors.New("session 未关联 store")
	}
	return s.store.setPending(s, toolName, argsJSON)
}

// ConsumePending 校验并消费待确认调用(handler 确认路径调用)。
func (s *Session) ConsumePending(toolName, token string) (string, error) {
	if s.store == nil {
		return "", errors.New("session 未关联 store")
	}
	return s.store.consumePending(s, toolName, token)
}

// ConsumeConsumed 一次性取走"确认后执行"放行标记(实现 tools.SessionScope)。
func (s *Session) ConsumeConsumed(toolName, argsJSON string) bool {
	return s.ConsumeConsumedPending(toolName, argsJSON)
}

// Compact 会话历史压缩:超过 max 条时保留 system+最近 N 条,
// 中间插入压缩摘要占位,防止上下文无限膨胀。
// 借鉴 zcode compact:远古细节丢弃,近期事实保留。
func (st *SessionStore) Compact(s *Session, max int) int {
	if max <= 0 {
		max = 40
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(s.Messages) <= max {
		return 0
	}
	keep := max - 2
	dropped := len(s.Messages) - keep
	summary := Message{
		Role:    "system",
		Content: "[上下文已压缩] 更早 " + textutil.Itoa(dropped) + " 条对话已省略,仅保留关键结论。",
	}
	tail := append([]Message{summary}, s.Messages[len(s.Messages)-keep:]...)
	s.Messages = tail
	return dropped
}
