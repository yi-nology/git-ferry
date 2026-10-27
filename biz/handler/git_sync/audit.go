package git_sync

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/oekit/logx"
	"github.com/yi-nology/git-ferry/internal/corebridge"
)

// logger 是本包的日志出口(经 logx 门面)。
//
// **用 Delegating 而不是 Default()**:包级 var 在 init 时求值,
// 直接捕获 Default() 会永远绑在 init 那一刻的默认 logger 上,
// 消费方随后的 SetDefault 对它无效;Delegating 是**调用时**解析,
// 自动跟随 main() 里装好的后端。
var logger = logx.Delegating("git-sync")

// recordAudit 记录一条审计日志（best-effort：写入失败仅告警，不影响主流程）。
//
// actor 优先级：
//  1. 壳层鉴权中间件写入的身份（SetAuthUser，内网 SSO/网关头）
//  2. 请求头 X-User（兼容旧调用方/脚本）
//  3. "admin"（共享 API Key 且未带用户头时的缺省）
//
// core 不感知登录态，只持久化 OperationLog.Actor。
func recordAudit(ctx context.Context, c *app.RequestContext, action, resourceType, resourceKey, resource string) {
	recordAuditDetail(ctx, c, action, resourceType, resourceKey, resource, "")
}

// recordAuditDetail 记录一条带明细的审计日志。
//
// Detail 承载结构化补充信息(如历史同步的 base_head/head/run id),
// 它同样进入哈希链 payload —— 明细被改也会被检出。
func recordAuditDetail(ctx context.Context, c *app.RequestContext, action, resourceType, resourceKey, resource, detail string) {
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
		Detail:       detail,
	}
	if svc := GetSyncService(); svc != nil {
		if err := svc.RecordOperation(ctx, entry); err != nil {
			logger.Warn("record audit log failed", "error", err, "action", action, "resource", resource)
		}
	} else {
		logger.Warn("record audit log skipped: service unavailable", "action", action)
	}
}
