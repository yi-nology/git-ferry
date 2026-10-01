package githubapi

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/yi-nology/git-ferry/internal/metrics"
)

// throttle 封装平台 API 限流退避：识别 403/429 + Retry-After / X-RateLimit-Remaining=0，
// 指数退避重试；观测写入 metrics（gitferry_api_ratelimit_total）。
//
// 退避序列：1s → 2s → 4s → … 上限 60s，最多 maxAttempts 次（默认 5）。

const (
	defaultMaxAttempts = 5
	defaultBaseDelay   = time.Second
	defaultMaxDelay    = time.Minute
)

// RateLimitEvent 记录一次限流退避，供调用方写入执行历史。
type RateLimitEvent struct {
	Endpoint  string
	Status    int
	Reason    string
	Backoff   time.Duration
	Attempt   int
	Recovered bool
}

// Throttler 带退避的 HTTP 执行器。
type Throttler struct {
	Client      *http.Client
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	// OnEvent 每次触发限流退避时回调（可选）。
	OnEvent func(RateLimitEvent)
	// Sleep 覆盖等待实现（测试用）；默认 time.Sleep。
	Sleep func(context.Context, time.Duration) error
}

// NewThrottler 默认节流器（60s 超时）。
func NewThrottler() *Throttler {
	return &Throttler{
		Client:      &http.Client{Timeout: 60 * time.Second},
		MaxAttempts: defaultMaxAttempts,
		BaseDelay:   defaultBaseDelay,
		MaxDelay:    defaultMaxDelay,
	}
}

// Do 执行请求；限流时退避重试。调用方负责准备 req 与关闭最终 resp.Body。
func (t *Throttler) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if t.Client == nil {
		t.Client = &http.Client{Timeout: 60 * time.Second}
	}
	attempts := t.MaxAttempts
	if attempts <= 0 {
		attempts = defaultMaxAttempts
	}
	base := t.BaseDelay
	if base <= 0 {
		base = defaultBaseDelay
	}
	maxDelay := t.MaxDelay
	if maxDelay <= 0 {
		maxDelay = defaultMaxDelay
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		// 每次重试需要新的 Body；这里仅支持 NoBody/Get，POST 调用方应自备 GetBody。
		resp, err := t.Client.Do(req)
		if err != nil {
			return nil, err
		}
		if !isRateLimited(resp) {
			return resp, nil
		}
		reason, backoff := parseRateLimit(resp, base, maxDelay, attempt)
		_ = resp.Body.Close()
		metrics.Default().AddLabeled("gitferry_api_ratelimit_total", "Platform API rate-limit backoffs",
			map[string]string{"host": req.URL.Host, "status": strconv.Itoa(resp.StatusCode)}, 1)
		ev := RateLimitEvent{
			Endpoint: req.URL.Path,
			Status:   resp.StatusCode,
			Reason:   reason,
			Backoff:  backoff,
			Attempt:  attempt,
		}
		if t.OnEvent != nil {
			t.OnEvent(ev)
		}
		lastErr = fmt.Errorf("rate limited: %s (status %d)", reason, resp.StatusCode)
		if attempt == attempts {
			break
		}
		if err := t.sleep(ctx, backoff); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

func (t *Throttler) sleep(ctx context.Context, d time.Duration) error {
	if t.Sleep != nil {
		return t.Sleep(ctx, d)
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isRateLimited(resp *http.Response) bool {
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if resp.StatusCode != http.StatusForbidden {
		return false
	}
	// GitHub secondary rate limit / primary exhausted 常见于 403。
	if resp.Header.Get("Retry-After") != "" {
		return true
	}
	if resp.Header.Get("X-RateLimit-Remaining") == "0" {
		return true
	}
	// 某些网关只给 X-RateLimit-Resource
	return resp.Header.Get("X-RateLimit-Resource") != "" &&
		resp.Header.Get("X-RateLimit-Remaining") == "0"
}

// parseRateLimit 返回 reason 与本次退避时长（指数：base*2^(attempt-1)，封顶 maxDelay）。
// Retry-After 秒数优先。
func parseRateLimit(resp *http.Response, base, maxDelay time.Duration, attempt int) (string, time.Duration) {
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil && secs > 0 {
			d := time.Duration(secs) * time.Second
			if d > maxDelay {
				d = maxDelay
			}
			return "retry-after=" + ra, d
		}
	}
	backoff := base
	for i := 1; i < attempt; i++ {
		backoff *= 2
		if backoff >= maxDelay {
			backoff = maxDelay
			break
		}
	}
	if backoff > maxDelay {
		backoff = maxDelay
	}
	reason := "http_" + strconv.Itoa(resp.StatusCode)
	if resp.Header.Get("X-RateLimit-Remaining") == "0" {
		reason = "x-ratelimit-remaining=0"
	}
	return reason, backoff
}
