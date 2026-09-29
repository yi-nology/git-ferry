package git_sync

import (
	"context"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
)

// Role 访问角色(RBAC 最小集)。
type Role string

const (
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleReadonly Role = "readonly"
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
	switch Role(strings.ToLower(strings.TrimSpace(s))) {
	case RoleAdmin:
		return RoleAdmin
	case RoleOperator:
		return RoleOperator
	default:
		return RoleReadonly
	}
}

// requireWriteRole 写操作放行:admin/operator。
func requireWriteRole(c *app.RequestContext) bool {
	r := GetAuthRole(c)
	if r == RoleAdmin || r == RoleOperator {
		return true
	}
	c.JSON(403, map[string]string{"error": "forbidden: role " + string(r) + " cannot write"})
	c.Abort()
	return false
}

// requireAdminRole 危险/治理操作放行:仅 admin。
func requireAdminRole(c *app.RequestContext) bool {
	if GetAuthRole(c) == RoleAdmin {
		return true
	}
	c.JSON(403, map[string]string{"error": "forbidden: admin role required"})
	c.Abort()
	return false
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
