package tools

import "context"

// SessionScope 危险工具所需的会话能力(agent.Session 实现之;测试用 fake)。
// 定义在 tools 侧保持依赖方向 tools ← agent 单向。
type SessionScope interface {
	SetPending(toolName, argsJSON string) (token string, err error)
	// ConsumeConsumed 一次性读取并清除放行标记:确认后仅放行一次执行。
	ConsumeConsumed(toolName, argsJSON string) bool
	MarkConsumed(toolName, argsJSON string)
	SetLastConfirm(toolName, token, argsJSON string)
	LastConfirm() (toolName, token, argsJSON string)

	// SetPermissionMode 记录本会话的工具权限模式。
	//
	// 返回 false 表示这条链路**存不住**(如无状态的 MCP 端点)——
	// 调用方据此如实告诉用户「这里切不了」,而不是假装切成功了。
	SetPermissionMode(mode string) bool
	// PermissionMode 取本会话显式设过的权限模式;空串表示没设过
	// (此时用进程级默认档)。
	PermissionMode() string
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
