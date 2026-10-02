package git_sync

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/yi-nology/git-ferry/internal/metrics"
	sdkprov "github.com/yi-nology/go-git-platform/provider"
)

// ProviderHooks 返回 core provider 生命周期钩子(平台 API 限流指标)。
// 经 corebridge.WithProviderHooks 传入 NewService,钩子随 provider 构造固化,
// 不再依赖"先于 core 初始化调用"的时序约定。
//
// 平台 transport 已接管 429/5xx 及 403+X-RateLimit-Remaining=0 的退避重试;
// 这里只负责观测:每次响应命中限流特征即记一次
// gitferry_api_ratelimit_total(含重试的每次尝试,与原 githubapi.Throttler
// "每次退避打点"近似)。
func ProviderHooks() *sdkprov.Hooks {
	return &sdkprov.Hooks{
		Response: []sdkprov.ResponseHook{rateLimitResponseHook},
	}
}

func rateLimitResponseHook(_ context.Context, req *http.Request, resp *http.Response, _ time.Duration, _ error) {
	if req == nil || resp == nil {
		return
	}
	if !isRateLimitResponse(resp) {
		return
	}
	host := ""
	if req.URL != nil {
		host = req.URL.Host
	}
	metrics.Default().AddLabeled("gitferry_api_ratelimit_total", "Platform API rate-limit backoffs",
		map[string]string{"host": host, "status": strconv.Itoa(resp.StatusCode)}, 1)
}

// isRateLimitResponse 与原 githubapi.isRateLimited 的打点口径一致:
// 429,或 403 且 X-RateLimit-Remaining=0(主限流耗尽)。
func isRateLimitResponse(resp *http.Response) bool {
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	return resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0"
}
