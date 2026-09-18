package agent

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// PendingConfirm 危险工具的一次待确认调用。确认令牌 = 哈希(会话+工具+参数),
// 前端原样带回;后端校验工具名与令牌一致且未过期后才执行,一次性消费。
type PendingConfirm struct {
	ToolName  string
	ArgsJSON  string
	Token     string
	ExpiresAt time.Time
}

const pendingTTL = 5 * time.Minute

var (
	ErrNoPending       = errors.New("ai_confirm_none")
	ErrConfirmMismatch = errors.New("ai_confirm_mismatch")
	ErrConfirmExpired  = errors.New("ai_confirm_expired")
)

// setPending 在会话上登记一次待确认调用(新调用覆盖旧调用),返回确认令牌。
func (st *SessionStore) setPending(s *Session, toolName, argsJSON string) (string, error) {
	if toolName == "" || argsJSON == "" {
		return "", fmt.Errorf("SetPending: 工具名与参数不能为空")
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	s.pending = &PendingConfirm{
		ToolName:  toolName,
		ArgsJSON:  argsJSON,
		Token:     confirmToken(s.ID, toolName, argsJSON),
		ExpiresAt: time.Now().Add(pendingTTL),
	}
	s.LastAt = time.Now()
	return s.pending.Token, nil
}

// consumePending 校验并消费待确认调用。任一不匹配返回错误(含一次性语义)。
func (st *SessionStore) consumePending(s *Session, toolName, token string) (string, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if s.pending == nil {
		return "", ErrNoPending
	}
	pc := s.pending
	s.pending = nil // 无论成败都消费,防重放
	if pc.ToolName != toolName || subtle.ConstantTimeCompare([]byte(pc.Token), []byte(token)) != 1 {
		return "", ErrConfirmMismatch
	}
	if time.Now().After(pc.ExpiresAt) {
		return "", ErrConfirmExpired
	}
	return pc.ArgsJSON, nil
}

// PendingSnapshot 只读拷贝当前待确认调用(测试用)。
func (s *Session) PendingSnapshot() *PendingConfirm {
	if s.store == nil {
		return nil
	}
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	if s.pending == nil {
		return nil
	}
	pc := *s.pending
	return &pc
}

// SetLastConfirm 记录最近一次确认请求(装饰器据此发 tool_confirm 事件)。
func (s *Session) SetLastConfirm(toolName, token, argsJSON string) {
	s.muConfirm.Lock()
	defer s.muConfirm.Unlock()
	s.lastConfirm = &PendingConfirm{ToolName: toolName, Token: token, ArgsJSON: argsJSON}
}

// LastConfirm 取走最近确认请求(取即清空),三返回值满足 tools.SessionScope。
func (s *Session) LastConfirm() (toolName, token, argsJSON string) {
	s.muConfirm.Lock()
	defer s.muConfirm.Unlock()
	if s.lastConfirm == nil {
		return "", "", ""
	}
	pc := *s.lastConfirm
	s.lastConfirm = nil
	return pc.ToolName, pc.Token, pc.ArgsJSON
}

// HasConsumedPending 判断"确认后执行":handler 已 consume 成功并调用
// MarkConsumed 后,危险工具第二次收到同参数请求时放行真实执行。
func (s *Session) HasConsumedPending(toolName, argsJSON string) bool {
	s.muConfirm.Lock()
	defer s.muConfirm.Unlock()
	return s.consumedTool == toolName && s.consumedArgs == argsJSON
}

// MarkConsumed 由 handler 在 ConsumePending 成功后调用。
func (s *Session) MarkConsumed(toolName, argsJSON string) {
	s.muConfirm.Lock()
	defer s.muConfirm.Unlock()
	s.consumedTool, s.consumedArgs = toolName, argsJSON
}

func confirmToken(sessionID, toolName, argsJSON string) string {
	sum := sha256.Sum256([]byte(sessionID + "\x00" + toolName + "\x00" + argsJSON))
	return hex.EncodeToString(sum[:])[:16]
}
