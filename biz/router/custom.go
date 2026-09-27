package router

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	handler "github.com/yi-nology/git-ferry/biz/handler"
	"github.com/yi-nology/git-ferry/biz/handler/git_sync"
	routergitsync "github.com/yi-nology/git-ferry/biz/router/git_sync"
	"github.com/yi-nology/git-ferry/internal/metrics"
	"github.com/yi-nology/git-ferry/internal/pkg/swagger"
)

// CustomizedRegister 注册非 IDL 生成的定制路由（探活、webhook、指标、运维补偿）。
// 公网壳与内网壳共用；hz 重新生成 main 侧 customizedRegister 时应转调此处。
func CustomizedRegister(r *server.Hertz) {
	// Liveness probe (no auth, no middleware)
	r.GET("/ping", handler.Ping)

	// Readiness probe (checks database and Redis, no auth)
	r.GET("/health", git_sync.HealthCheck)

	// Prometheus 指标(公开,供抓取端拉取)
	r.GET("/metrics", metricsHandler)

	// Webhook 接收端点（带速率限制，不走 API 鉴权）
	r.POST("/api/webhook/receive/:repoKey", git_sync.RateLimitMiddleware(), git_sync.ReceiveWebhook)

	// AI 助手（未启用时 handler 返回 501;鉴权与业务 API 同强度）
	ai := r.Group("/api/v1/ai", routergitsync.AuthMiddleware())
	ai.GET("/status", git_sync.AIStatus)
	ai.POST("/chat", git_sync.AIChat)

	// 失败补偿与治理(鉴权)
	ops := r.Group("/api/v1/ops", routergitsync.AuthMiddleware())
	ops.POST("/retry", git_sync.RetryRun)
	ops.POST("/retry-batch", git_sync.BatchRetryFailed)
	ops.GET("/health-score", git_sync.HealthScore)
	ops.GET("/overview", git_sync.SyncOverview)
	ops.GET("/audit-report", git_sync.AuditReport)

	// Swagger API 文档(公开,无需鉴权)
	r.GET("/swagger/", swagger.SwaggerUI)
	r.GET("/swagger/index.html", swagger.SwaggerUI)
	r.GET("/swagger/openapi.json", swagger.OpenAPISpec)
}

// metricsHandler 输出 Prometheus 文本格式指标。
func metricsHandler(_ context.Context, c *app.RequestContext) {
	c.Data(consts.StatusOK, "text/plain; version=0.0.4; charset=utf-8",
		[]byte(metrics.Default().WritePrometheus()))
}
