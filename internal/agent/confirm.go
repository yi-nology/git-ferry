package agent

import (
	"crypto/rand"
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
		Token:     confirmToken(),
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

// ConsumeConsumedPending 一次性读取并清除"确认后执行"放行标记。
// 确认直达执行会再走一次 dangerGuard:本次放行真实执行;之后同参数再调
// 必须重新确认,避免会话内免确认重放。
func (s *Session) ConsumeConsumedPending(toolName, argsJSON string) bool {
	s.muConfirm.Lock()
	defer s.muConfirm.Unlock()
	if s.consumedTool == toolName && s.consumedArgs == argsJSON {
		s.consumedTool, s.consumedArgs = "", ""
		return true
	}
	return false
}

// MarkConsumed 由 handler 在 ConsumePending 成功后调用。
func (s *Session) MarkConsumed(toolName, argsJSON string) {
	s.muConfirm.Lock()
	defer s.muConfirm.Unlock()
	s.consumedTool, s.consumedArgs = toolName, argsJSON
}

// confirmToken 生成不可预测的一次性确认令牌(128 bit 随机)。
// 确定性哈希会被同会话内已知参数推算,不可用作确认凭证。
func confirmToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand 在目标平台失败属致命错误,宁可 panic 也不降级为弱令牌
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}
