package tools

import "context"

// ClientConfirmedScope 是给**外部 MCP 客户端**用的 scope。
//
// ## 为什么 MCP 不走「登记待确认 → 用户确认 → 执行」
//
// 「登记待确认」这套机制是为**本进程内的 agent** 设计的:模型在同一个
// 会话里发起工具调用，服务端登记一个待确认项，前端把令牌带回来，
// 模型用令牌再调一次才执行。
//
// MCP 是**无状态**的:客户端（Claude Code / Cursor）在把工具调用发过来之前，
// **它自己就已经向用户展示并征求过同意**了 —— 确认发生在客户端那一侧。
// 服务端这边再登记一个「待确认」，登记与执行根本不在同一个会话里，
// 令牌无处可去 —— 结果就是危险工具**在 MCP 上根本不可用**
// （会话上下文缺失，直接拒绝）。
//
// ## 它不绕过什么
//
//   - **角色闸门照旧**：`dangerGuard` 的角色检查在 scope 检查**之前**
//     （fail-closed）—— 只读角色在 MCP 上同样执行不了写工具。
//   - **强制推送的 admin 闸门照旧**：那是 core 层的批准者校验，MCP 也一样。
//   - **审计链照旧**：每次执行照样入链、可校验。
//
// ## 口径对齐
//
// MCP 路径与 HTTP 路径一致（都是「客户端负责确认，服务端负责角色」）；
// CLI 与内置 AI 助手是本进程 agent，服务端必须自己加确认闸 —— 那条差异有理由。
type ClientConfirmedScope struct{}

// SetPending 不登记任何待确认项：MCP 的确认在客户端侧完成了。
func (ClientConfirmedScope) SetPending(string, string) (string, error) { return "", nil }

// ConsumeConsumed 恒为 true：视为客户端已确认，直接放行。
func (ClientConfirmedScope) ConsumeConsumed(string, string) bool { return true }

// MarkConsumed 无状态可标记。
func (ClientConfirmedScope) MarkConsumed(string, string) {}

// SetLastConfirm 无状态可记录。
func (ClientConfirmedScope) SetLastConfirm(string, string, string) {}

// LastConfirm 无状态可读。
func (ClientConfirmedScope) LastConfirm() (string, string, string) { return "", "", "" }

// SetPermissionMode 存不住:MCP 是无状态的,没有「会话」可挂。
//
// 返回 false 是**如实**而不是拒绝:调用方会告诉用户
// 「权限模式是会话级设置,MCP 端点上切不了」。
func (ClientConfirmedScope) SetPermissionMode(string) bool { return false }

// PermissionMode 没有会话状态,恒为空(⇒ 用进程级默认档)。
func (ClientConfirmedScope) PermissionMode() string { return "" }

// WithClientConfirmedScope 给 ctx 装上「客户端已确认」的 scope。
//
// 用途:MCP 端点（以及任何「确认由调用方负责」的外部链路）。
func WithClientConfirmedScope(ctx context.Context) context.Context {
	return WithScope(ctx, ClientConfirmedScope{})
}
