package git_sync

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
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
		// 写入不可伪造的审计身份:共享 API Key 场景下客户端 X-User 可信度为零,
		// 若不在此覆盖,任何持 key 的调用方都能冒充任意操作人。
		handler.SetAuthUser(c, "api-key:"+keyFingerprint(serverKey))
		c.Next(ctx)
	}
}

// keyFingerprint 取 API Key 指纹(非密钥本体),作审计 actor 标识。
// 不写入完整 key,避免日志泄密;同 key 在不同环境可区分。
func keyFingerprint(key string) string {
	if key == "" {
		return "empty"
	}
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])[:8]
}
