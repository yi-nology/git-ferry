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
	ai.GET("/config", git_sync.AIGetConfig)
	ai.POST("/config", git_sync.AIUpdateConfig)
	ai.POST("/config/test", git_sync.AITestConfig)
	ai.GET("/models", git_sync.AIListModels)

	// 失败补偿与治理(鉴权 + 写操作 RBAC)
	ops := r.Group("/api/v1/ops", routergitsync.AuthMiddleware())
	ops.GET("/health-score", git_sync.HealthScore)
	ops.GET("/overview", git_sync.SyncOverview)
	ops.GET("/audit-report", git_sync.AuditReport)
	ops.GET("/audit-chain/verify", git_sync.VerifyAuditChain)
	ops.GET("/rbac", git_sync.GetRBAC)
	ops.GET("/templates", git_sync.ListTemplates)
	ops.POST("/templates", git_sync.WriteGuard(), git_sync.CreateTemplate)
	ops.POST("/templates/delete", git_sync.WriteGuard(), git_sync.DeleteTemplate)
	ops.POST("/templates/preview", git_sync.PreviewTemplate)
	ops.POST("/templates/apply", git_sync.AdminGuard(), git_sync.ApplyTemplate)
	ops.GET("/inventory", git_sync.RepoInventory)
	ops.POST("/deploy-key", git_sync.AdminGuard(), git_sync.GenerateDeployKey)
	ops.GET("/issues-export", git_sync.ExportIssues)
	ops.POST("/rebuild", git_sync.AdminGuard(), git_sync.RebuildRepo)
	ops.POST("/migration", git_sync.AdminGuard(), git_sync.ExportGitHubMigration)
	ops.POST("/sync-platform", git_sync.WriteGuard(), git_sync.SyncPlatformFiltered)
	ops.GET("/bundles", git_sync.ListBundles)
	ops.GET("/bundles/verify", git_sync.VerifyBundle)
	ops.POST("/bundles/restore", git_sync.AdminGuard(), git_sync.RestoreBundle)
	ops.GET("/diagnose", git_sync.DiagnoseRun)
	ops.POST("/retry", git_sync.WriteGuard(), git_sync.RetryRun)
	ops.POST("/retry-batch", git_sync.WriteGuard(), git_sync.BatchRetryFailed)

	// P0 灾备闭环:DR 演练 / 完整性证明 / RPO-RTO
	ops.POST("/dr-drill", git_sync.WriteGuard(), git_sync.RunDRDrill)
	ops.GET("/dr-drill/history", git_sync.DrillHistory)
	ops.GET("/dr-drill/export", git_sync.ExportDrillHistory)
	ops.GET("/dr-drill/chain/verify", git_sync.VerifyDrillChain)
	ops.POST("/backup-manifest", git_sync.WriteGuard(), git_sync.BuildBackupManifest)
	ops.GET("/backup-manifest/verify", git_sync.VerifyBackupManifest)
	ops.GET("/rpo", git_sync.RPOReport)

	// P1 元数据资产:issues/PR/releases 快照 + source archive + Release 附件 + gists
	ops.POST("/metadata-backup", git_sync.WriteGuard(), git_sync.MetadataBackup)
	ops.GET("/metadata-backups", git_sync.ListMetadataBackups)
	ops.POST("/gists-backup", git_sync.WriteGuard(), git_sync.BackupGists)

	// P3 生命周期:自动发现 / 漂移检测 / 冷备清理
	ops.POST("/auto-discover", git_sync.WriteGuard(), git_sync.AutoDiscover)
	ops.POST("/drift", git_sync.DetectDrift)
	ops.POST("/backup-cleanup", git_sync.AdminGuard(), git_sync.CleanupBackups)

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
