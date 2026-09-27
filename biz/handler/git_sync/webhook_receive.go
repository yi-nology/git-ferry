package git_sync

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	sdkprov "github.com/yi-nology/go-git-platform/provider"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
	"golang.org/x/time/rate"
)

const maxWebhookBodySize = 10 << 20

// webhookDefaultRate 默认每 IP 每秒请求数(配置为 0 或负数时)。
const webhookDefaultRate = 10

// ipRateLimiter 按客户端 IP 分桶的令牌桶限流。
// 单桶全进程共享会被单一来源打满,合法来源跟着遭殃;按 IP 分桶后互不影响。
type ipRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	rate     rate.Limit
	burst    int
	// lastSeen 用于惰性清理,防止 map 随来源 IP 无限增长
	lastSeen map[string]time.Time
}

func newIPRateLimiter(perSecond int) *ipRateLimiter {
	if perSecond <= 0 {
		perSecond = webhookDefaultRate
	}
	return &ipRateLimiter{
		limiters: make(map[string]*rate.Limiter),
		lastSeen: make(map[string]time.Time),
		rate:     rate.Limit(perSecond),
		burst:    perSecond,
	}
}

// Allow 对 key(通常为 ClientIP)做限流判断;超限返回 false。
func (l *ipRateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	lim, ok := l.limiters[key]
	if !ok {
		lim = rate.NewLimiter(l.rate, l.burst)
		l.limiters[key] = lim
	}
	l.lastSeen[key] = time.Now()
	// 超过 256 个 key 时清理 10 分钟未活动的桶,控制内存
	if len(l.limiters) > 256 {
		cutoff := time.Now().Add(-10 * time.Minute)
		for k, t := range l.lastSeen {
			if t.Before(cutoff) {
				delete(l.limiters, k)
				delete(l.lastSeen, k)
			}
		}
	}
	return lim.Allow()
}

// allowAt 在指定时间点判定(测试用:模拟令牌回填,无需真实等待)。
func (l *ipRateLimiter) allowAt(key string, at time.Time, n int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	lim, ok := l.limiters[key]
	if !ok {
		lim = rate.NewLimiter(l.rate, l.burst)
		l.limiters[key] = lim
	}
	return lim.AllowN(at, n)
}

var (
	webhookRateLimiter   *ipRateLimiter
	webhookRateLimiterMu sync.Mutex
)

// getWebhookRateLimiter returns the per-IP rate limiter, initializing it from config on first call.
func getWebhookRateLimiter() *ipRateLimiter {
	webhookRateLimiterMu.Lock()
	defer webhookRateLimiterMu.Unlock()
	if webhookRateLimiter == nil {
		rateLimit := webhookDefaultRate
		if svc := GetSyncService(); svc != nil {
			if cfg := svc.GetConfig(); cfg != nil && cfg.Webhook.RateLimit > 0 {
				rateLimit = cfg.Webhook.RateLimit
			}
		}
		webhookRateLimiter = newIPRateLimiter(rateLimit)
	}
	return webhookRateLimiter
}

// setWebhookRateLimiter overrides the rate limiter. Used for testing.
func setWebhookRateLimiter(rl *ipRateLimiter) {
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

// RateLimitMiddleware 按客户端 IP 限流;超限返回 429。
func RateLimitMiddleware() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		if !getWebhookRateLimiter().Allow(c.ClientIP()) {
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
	if svc := GetSyncService(); svc != nil {
		if cfg := svc.GetConfig(); cfg != nil && cfg.Webhook.MaxBodySize > 0 {
			bodySizeLimit = cfg.Webhook.MaxBodySize
		}
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

	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	err := svc.ReceiveWebhook(ctx, repoKey, &corebridge.WebhookPayload{
		Method:     string(c.Method()),
		Path:       string(c.Path()),
		Header:     header,
		Body:       bodyBytes,
		RemoteAddr: c.ClientIP(),
	})
	if err != nil {
		// 签名验证失败是认证错误,返 401 而非 500;细节记服务端日志,不暴露给调用方
		if sdkprov.IsWebhookValidation(err) {
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
