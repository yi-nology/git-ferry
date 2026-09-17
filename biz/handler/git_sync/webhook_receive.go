package git_sync

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
	"golang.org/x/time/rate"
)

const maxWebhookBodySize = 10 << 20

// newRateLimiter 创建限流器(令牌桶,容量与速率均为 ratePerSecond/秒;0 表示全拒)。
// 基于 golang.org/x/time/rate,替代此前手写的令牌桶实现。
func newRateLimiter(ratePerSecond int) *rate.Limiter {
	return rate.NewLimiter(rate.Limit(ratePerSecond), ratePerSecond)
}

var (
	webhookRateLimiter   *rate.Limiter
	webhookRateLimiterMu sync.Mutex
)

// getWebhookRateLimiter returns the singleton rate limiter, initializing it from config on first call.
func getWebhookRateLimiter() *rate.Limiter {
	webhookRateLimiterMu.Lock()
	defer webhookRateLimiterMu.Unlock()
	if webhookRateLimiter == nil {
		rateLimit := GetSyncService().GetConfig().Webhook.RateLimit
		if rateLimit <= 0 {
			rateLimit = 10 // default: 10 requests per second
		}
		webhookRateLimiter = newRateLimiter(rateLimit)
	}
	return webhookRateLimiter
}

// setWebhookRateLimiter overrides the rate limiter. Used for testing.
func setWebhookRateLimiter(rl *rate.Limiter) {
	webhookRateLimiterMu.Lock()
	defer webhookRateLimiterMu.Unlock()
	webhookRateLimiter = rl
}

// resetWebhookRateLimiter resets the rate limiter so it will be re-initialized from config. Used for testing.
func resetWebhookRateLimiter() {
	webhookRateLimiterMu.Lock()
	defer webhookRateLimiterMu.Unlock()
	webhookRateLimiter = nil
}

// RateLimitMiddleware returns a middleware that enforces webhook rate limiting.
// When the rate limit is exceeded, it responds with HTTP 429 Too Many Requests.
func RateLimitMiddleware() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		if !getWebhookRateLimiter().Allow() {
			response.Error(c, consts.StatusTooManyRequests, "rate limit exceeded, please try again later")
			c.Abort()
			return
		}
		c.Next(ctx)
	}
}

func ReceiveWebhook(ctx context.Context, c *app.RequestContext) {
	repoKey := c.Param("repoKey")
	if repoKey == "" {
		response.BadRequest(c, "repoKey is required")
		return
	}

	// Use configured max body size, fallback to default 10MB
	bodySizeLimit := maxWebhookBodySize
	if cfg := GetSyncService().GetConfig(); cfg != nil && cfg.Webhook.MaxBodySize > 0 {
		bodySizeLimit = cfg.Webhook.MaxBodySize
	}

	// 先检查 Content-Length 头,快速拒绝过大的请求(避免读入内存)
	if contentLen := c.Request.Header.ContentLength(); contentLen > bodySizeLimit {
		response.Error(c, consts.StatusRequestEntityTooLarge, "request body too large")
		return
	}

	bodyBytes, bodyErr := c.Body()
	if bodyErr != nil {
		response.BadRequest(c, "failed to read request body")
		return
	}
	if len(bodyBytes) > bodySizeLimit {
		response.Error(c, consts.StatusRequestEntityTooLarge, "request body too large")
		return
	}

	// 壳层负责 HTTP 协议；core 只收协议无关载荷，不在库内起 Web 服务。
	header := make(map[string][]string)
	c.Request.Header.VisitAll(func(k, v []byte) {
		header[string(k)] = append(header[string(k)], string(v))
	})

	err := GetSyncService().ReceiveWebhook(ctx, repoKey, &corebridge.WebhookPayload{
		Method:     string(c.Method()),
		Path:       string(c.Path()),
		Header:     header,
		Body:       bodyBytes,
		RemoteAddr: c.ClientIP(),
	})
	if err != nil {
		// 签名验证失败是认证错误,返 401 而非 500;细节记服务端日志,不暴露给调用方
		if strings.Contains(err.Error(), "signature") || strings.Contains(err.Error(), "unauthorized") {
			slog.Warn("webhook signature verification failed", "repo", repoKey, "error", err, "client_ip", c.ClientIP())
			response.Error(c, consts.StatusUnauthorized, "invalid webhook signature")
		} else {
			response.InternalError(c, err.Error())
		}
		return
	}

	response.Success(c, map[string]any{
		"message": "webhook received",
	})
}
