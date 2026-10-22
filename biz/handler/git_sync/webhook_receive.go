package git_sync

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/oekit/concurrency/ratelimit"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
	sdkprov "github.com/yi-nology/go-git-platform/provider"
	"golang.org/x/time/rate"
)

const maxWebhookBodySize = 10 << 20

// webhookDefaultRate 默认每 IP 每秒请求数(配置为 0 或负数时)。
const webhookDefaultRate = 10

// webhookLimiterIdleTTL 限流桶的空闲回收时长:超过该时长未活动的桶被回收,
// 控制按 IP 分桶的内存占用(与原手写实现的惰性清理同口径)。
const webhookLimiterIdleTTL = 10 * time.Minute

// ipRateLimiter 按客户端 IP 分桶的令牌桶限流。
// 单桶全进程共享会被单一来源打满,合法来源跟着遭殃;按 IP 分桶后互不影响。
type ipRateLimiter struct {
	keyed *ratelimit.KeyedLimiterOf[string]
	rate  rate.Limit
	burst int
	// fakeNow 非 nil 时令牌桶按该时刻判定配额(测试用:不靠真实等待驱动回填)。
	fakeNow atomic.Pointer[time.Time]
}

func newIPRateLimiter(perSecond int) *ipRateLimiter {
	if perSecond <= 0 {
		perSecond = webhookDefaultRate
	}
	l := &ipRateLimiter{
		rate:  rate.Limit(perSecond),
		burst: perSecond,
	}
	l.keyed = ratelimit.NewKeyedLimiterByFunc[string](
		func(string) ratelimit.Limiter {
			return ratelimit.NewTokenBucket(l.rate, l.burst, ratelimit.WithTokenClock(l.clock))
		},
		// 不设 MaxEntries(硬上限):key 来自 ClientIP,而 hertz 默认信任
		// X-Forwarded-For/X-Real-IP(默认可信网段是 0.0.0.0/0),key 空间可被伪造。
		// 硬上限会让伪造流量占满表后反过来把合法 webhook 来源全部拒成 429。
		// 原实现本就没有硬上限、只有 10 分钟空闲回收——这里保持同一口径。
		ratelimit.WithIdleTTL[string](webhookLimiterIdleTTL),
	)
	return l
}

// clock 令牌桶的判定时刻:正常取当前时间,测试可注入固定时刻。
func (l *ipRateLimiter) clock() time.Time {
	if t := l.fakeNow.Load(); t != nil {
		return *t
	}
	return time.Now()
}

// Allow 对 key(通常为 ClientIP)做限流判断;超限返回 false。
func (l *ipRateLimiter) Allow(key string) bool {
	return l.keyed.Allow(key)
}

// allowAt 在指定时间点判定(测试用:模拟令牌回填,无需真实等待)。
// 与真实 Allow 共用同一个桶,只是把判定时刻换成 at——语义与原实现一致。
// 仅供测试使用:调用期间并发 Allow 会读到假时刻,生产路径不会走到这里。
func (l *ipRateLimiter) allowAt(key string, at time.Time, n int) bool {
	l.fakeNow.Store(&at)
	defer l.fakeNow.Store(nil)
	return l.keyed.AllowN(key, n)
}

// Close 停止限流器的后台回收 goroutine(可重复调用)。
func (l *ipRateLimiter) Close() {
	l.keyed.Close()
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
	old := webhookRateLimiter
	webhookRateLimiter = rl
	if old != nil && old != rl {
		old.Close()
	}
}

// resetWebhookRateLimiter resets the rate limiter so it will be re-initialized from config. Used for testing.
func resetWebhookRateLimiter() {
	webhookRateLimiterMu.Lock()
	defer webhookRateLimiterMu.Unlock()
	if webhookRateLimiter != nil {
		webhookRateLimiter.Close()
		webhookRateLimiter = nil
	}
}

// CloseWebhookRateLimiter 关闭 webhook 限流器的后台回收 goroutine。
// 服务退出路径应在 HTTP 排空后调用它(与 core.Service.Stop 同一批次)。
func CloseWebhookRateLimiter() {
	resetWebhookRateLimiter()
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
