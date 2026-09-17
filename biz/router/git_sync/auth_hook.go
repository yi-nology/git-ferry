package git_sync

import (
	"context"
	"crypto/subtle"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
	handler "github.com/yi-nology/git-ferry/biz/handler/git_sync"
)

// AuthMiddlewareProvider 允许壳层（公网 / 内网）替换默认 X-API-Key 鉴权，
// 例如内网 SSO、反向代理注入用户头、CAS/OIDC。返回 nil 时回退默认实现。
type AuthMiddlewareProvider func() app.HandlerFunc

var authProvider AuthMiddlewareProvider

// SetAuthMiddlewareProvider 注册自定义鉴权。须在路由注册（Register）之前调用。
func SetAuthMiddlewareProvider(p AuthMiddlewareProvider) {
	authProvider = p
}

// ResolveAuthMiddleware 供 AuthMiddleware 使用：优先壳层 Provider，否则默认 API Key。
func ResolveAuthMiddleware() app.HandlerFunc {
	if authProvider != nil {
		if mw := authProvider(); mw != nil {
			return mw
		}
	}
	return DefaultAPIKeyAuthMiddleware()
}

// DefaultAPIKeyAuthMiddleware 校验 X-API-Key（常量时间比较）。
// API Key 由壳层注入（handler.SetAPIKey），不经过 git-sync-core。
// 服务端 API Key 为空时拒绝全部请求。
func DefaultAPIKeyAuthMiddleware() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		serverKey := handler.GetAPIKey()
		if serverKey == "" {
			c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized: API key not configured"})
			c.Abort()
			return
		}
		apiKey := c.GetHeader("X-API-Key")
		if subtle.ConstantTimeCompare([]byte(apiKey), []byte(serverKey)) != 1 {
			c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			c.Abort()
			return
		}
		c.Next(ctx)
	}
}
