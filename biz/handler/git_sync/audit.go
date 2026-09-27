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
	// 优先已认证身份(鉴权中间件写入,不可伪造)。
	// X-User 仅在未走过鉴权中间件时兜底(兼容旧脚本),共享 API Key 场景下
	// DefaultAPIKeyAuthMiddleware 已写入 api-key:<指纹>,此处不会再读到伪造头。
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
	if svc := GetSyncService(); svc != nil {
		if err := svc.RecordOperation(ctx, entry); err != nil {
			slog.Warn("record audit log failed", "error", err, "action", action, "resource", resource)
		}
	} else {
		slog.Warn("record audit log skipped: service unavailable", "action", action)
	}
}
