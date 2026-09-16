package tools

import "context"

// SessionScope 危险工具所需的会话能力(agent.Session 实现之;测试用 fake)。
// 定义在 tools 侧保持依赖方向 tools ← agent 单向。
type SessionScope interface {
	SetPending(toolName, argsJSON string) (token string, err error)
	HasConsumed(toolName, argsJSON string) bool
	MarkConsumed(toolName, argsJSON string)
	SetLastConfirm(toolName, token, argsJSON string)
	LastConfirm() (toolName, token, argsJSON string)
}

type scopeKey struct{}

// WithScope 把会话能力注入 ctx(Runner 在每轮对话开始时调用)。
func WithScope(ctx context.Context, sc SessionScope) context.Context {
	return context.WithValue(ctx, scopeKey{}, sc)
}

// ScopeFrom 取会话能力;非 agent 链路为 nil。
func ScopeFrom(ctx context.Context) SessionScope {
	sc, _ := ctx.Value(scopeKey{}).(SessionScope)
	return sc
}
