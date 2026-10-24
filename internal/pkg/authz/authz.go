// Package authz 承载访问角色(RBAC 最小集)与它在 Go context 里的传递。
//
// ## 为什么要有这个包
//
// 角色原本只存在 hertz 的 RequestContext 上(`biz/handler/git_sync`)。
// 但**工具层要判角色**:MCP 端点与 AI 助手都是**进程内直调 core service**,
// 不过 HTTP 路由 —— 路由上的 WriteGuard/AdminGuard 对它们不生效。
//
// 之前写工具只是「因为 MCP 路径不注入会话 scope」才被挡住的 ——
// 那是偶然,不是设计:一旦有人注入 scope、或新增一个没套守卫的写工具,
// 天窗立刻成立。
//
// 所以角色必须能**跟着 ctx 走到工具层**。而工具包不能反向依赖
// biz/handler(会成环),于是角色类型与 ctx 存取都放在这里 ——
// 它是纯叶子包,谁都能引。
package authz

import "context"

// Role 访问角色。
type Role string

const (
	// RoleAdmin 管理员:可写、可治理、可强推。
	RoleAdmin Role = "admin"
	// RoleOperator 操作员:可写、可发布,不可强推覆盖与治理。
	RoleOperator Role = "operator"
	// RoleReadonly 只读:只能看。
	RoleReadonly Role = "readonly"
)

// ParseRole 解析角色字符串;非法回落 readonly(安全侧)。
func ParseRole(s string) Role {
	switch Role(s) {
	case RoleAdmin:
		return RoleAdmin
	case RoleOperator:
		return RoleOperator
	default:
		return RoleReadonly
	}
}

// CanWrite 判断角色能否执行写操作。
func CanWrite(r Role) bool {
	return r == RoleAdmin || r == RoleOperator
}

// IsAdmin 判断角色是否为管理员。
func IsAdmin(r Role) bool {
	return r == RoleAdmin
}

// ctxKey 是角色在 context 里的键(私有类型,避免与别的包撞键)。
type ctxKey struct{}

// WithRole 把角色放进 ctx。
func WithRole(ctx context.Context, r Role) context.Context {
	return context.WithValue(ctx, ctxKey{}, r)
}

// RoleFromContext 取 ctx 里的角色;**缺省 readonly(安全侧)**。
//
// 缺省安全侧意味着:任何没设角色的调用方都只能做只读的事 ——
// 新增入口忘了传角色,后果是「不能用」,而不是「什么都能干」。
func RoleFromContext(ctx context.Context) Role {
	if ctx == nil {
		return RoleReadonly
	}
	if r, ok := ctx.Value(ctxKey{}).(Role); ok && r != "" {
		return r
	}
	return RoleReadonly
}
