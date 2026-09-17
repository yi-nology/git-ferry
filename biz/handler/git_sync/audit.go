package git_sync

import (
	"context"
	"log/slog"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/internal/corebridge"
)

// recordAudit 记录一条审计日志（best-effort：写入失败仅告警，不影响主流程）。
//
// actor 优先级：
//  1. 壳层鉴权中间件写入的身份（SetAuthUser，内网 SSO/网关头）
//  2. 请求头 X-User（兼容旧调用方/脚本）
//  3. "admin"（共享 API Key 且未带用户头时的缺省）
//
// core 不感知登录态，只持久化 OperationLog.Actor。
func recordAudit(ctx context.Context, c *app.RequestContext, action, resourceType, resourceKey, resource string) {
	actor := GetAuthUser(c)
	if actor == "" {
		actor = string(c.GetHeader("X-User"))
	}
	if actor == "" {
		actor = "admin"
	}
	entry := &corebridge.OperationLog{
		Action:       action,
		ResourceType: resourceType,
		ResourceKey:  resourceKey,
		Resource:     resource,
		Actor:        actor,
		IP:           c.ClientIP(),
		Status:       corebridge.StatusSuccess,
	}
	if err := GetSyncService().RecordOperation(ctx, entry); err != nil {
		slog.Warn("record audit log failed", "error", err, "action", action, "resource", resource)
	}
}
