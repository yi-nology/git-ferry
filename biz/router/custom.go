package router

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	handler "github.com/yi-nology/git-ferry/biz/handler"
	"github.com/yi-nology/git-ferry/biz/handler/git_sync"
	routergitsync "github.com/yi-nology/git-ferry/biz/router/git_sync"
)

// CustomizedRegister 注册非 IDL 生成的定制路由（探活、webhook 接收等）。
// 公网壳与内网壳共用；hz 重新生成 main 侧 customizedRegister 时应转调此处。
func CustomizedRegister(r *server.Hertz) {
	// Liveness probe (no auth, no middleware)
	r.GET("/ping", handler.Ping)

	// Readiness probe (checks database and Redis, no auth)
	r.GET("/health", git_sync.HealthCheck)

	// Webhook 接收端点（带速率限制，不走 API 鉴权）
	r.POST("/api/webhook/receive/:repoKey", git_sync.RateLimitMiddleware(), git_sync.ReceiveWebhook)

	// AI 助手（未启用时 handler 返回 501;鉴权与业务 API 同强度）
	ai := r.Group("/api/v1/ai", routergitsync.AuthMiddleware())
	ai.GET("/status", git_sync.AIStatus)
	ai.POST("/chat", git_sync.AIChat)
}
