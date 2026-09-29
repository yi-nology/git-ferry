package git_sync

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
)

// VerifyAuditChain GET /api/v1/ops/audit-chain/verify
// 校验审计日志哈希链完整性(防篡改证明)。
func VerifyAuditChain(ctx context.Context, c *app.RequestContext) {
	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	res, err := svc.VerifyAuditChain()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, res)
}

// GetRBAC GET /api/v1/ops/rbac
// 返回当前调用方角色(供前端隐藏无权按钮)。
func GetRBAC(_ context.Context, c *app.RequestContext) {
	response.Success(c, map[string]any{
		"role": string(GetAuthRole(c)),
		"user": GetAuthUser(c),
		"permissions": map[string]bool{
			"read":  true,
			"write": GetAuthRole(c) == RoleAdmin || GetAuthRole(c) == RoleOperator,
			"admin": GetAuthRole(c) == RoleAdmin,
		},
	})
}
