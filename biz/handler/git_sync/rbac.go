package git_sync

import (
	"context"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/yi-nology/git-ferry/internal/pkg/authz"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// Role 访问角色(RBAC 最小集)。
//
// 别名到 authz 包:工具层要判角色,而工具包不能反向依赖 biz/handler
// (会成环)—— 角色类型与 ctx 传递都放在 internal/pkg/authz。
type Role = authz.Role

const (
	RoleAdmin    = authz.RoleAdmin
	RoleOperator = authz.RoleOperator
	RoleReadonly = authz.RoleReadonly
)

// roleKey 请求上下文角色键。
const roleKey = "git-ferry-role"

// SetAuthRole 写入角色(鉴权中间件调用)。
func SetAuthRole(c *app.RequestContext, r Role) {
	c.Set(roleKey, string(r))
}

// GetAuthRole 读取角色;缺省 readonly(安全侧)。
func GetAuthRole(c *app.RequestContext) Role {
	if v, ok := c.Get(roleKey); ok {
		if s, ok2 := v.(string); ok2 && s != "" {
			return Role(s)
		}
	}
	return RoleReadonly
}

// ParseRole 解析角色字符串;非法回落 readonly。
func ParseRole(s string) Role {
	return authz.ParseRole(strings.ToLower(strings.TrimSpace(s)))
}

// requireWriteRole 写操作放行:admin/operator。
func requireWriteRole(c *app.RequestContext) bool {
	r := GetAuthRole(c)
	if r == RoleAdmin || r == RoleOperator {
		return true
	}
	// **码由产生处定**(response.NewBizError),渲染处不猜 ——
	// 与其它业务错误走同一条出口(统一信封)。
	response.Biz(c, response.NewBizError(consts.StatusForbidden,
		"forbidden: role "+string(r)+" cannot write"))
	c.Abort()
	return false
}

// requireAdminRole 危险/治理操作放行:仅 admin。
func requireAdminRole(c *app.RequestContext) bool {
	if GetAuthRole(c) == RoleAdmin {
		return true
	}
	response.Biz(c, response.NewBizError(consts.StatusForbidden,
		"forbidden: admin role required"))
	c.Abort()
	return false
}

// CanWriteRole 判断角色能否执行写操作(admin/operator)。
//
// 给**绕过路由层**的调用方用(MCP 端点与 AI 助手直接调 core service,
// 不过路由的 WriteGuard)—— 那条路上没有中间件兜底,只能就地判。
func CanWriteRole(r Role) bool {
	return authz.CanWrite(r)
}

// WriteGuard 写操作 RBAC 中间件(挂到 POST/PUT/DELETE 路由)。
func WriteGuard() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		if !requireWriteRole(c) {
			return
		}
		c.Next(ctx)
	}
}

// AdminGuard 治理/危险操作 RBAC 中间件。
func AdminGuard() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		if !requireAdminRole(c) {
			return
		}
		c.Next(ctx)
	}
}
