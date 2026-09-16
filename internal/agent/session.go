package agent

import (
	"errors"
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

	muConfirm      sync.Mutex
	lastConfirm    *PendingConfirm // 最近一次发给前端的确认请求(取即清空)
	consumedTool   string          // 确认后直达执行的放行标记
	consumedArgs   string
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

// HasConsumed 判断"确认后执行"放行标记(实现 tools.SessionScope)。
func (s *Session) HasConsumed(toolName, argsJSON string) bool {
	return s.HasConsumedPending(toolName, argsJSON)
}
