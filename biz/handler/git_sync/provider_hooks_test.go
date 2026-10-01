package git_sync

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yi-nology/git-ferry/internal/metrics"
)

// TestRateLimitResponseHook 锁定限流指标打点口径(自原 githubapi/throttle.go 迁移):
// 429 或 403+X-RateLimit-Remaining=0 记一次,其它状态不动。
func TestRateLimitResponseHook(t *testing.T) {
	newReq := func() *http.Request {
		req, err := http.NewRequest(http.MethodGet, "https://hook-test.invalid/repos/o/r", http.NoBody)
		assert.NoError(t, err)
		return req
	}
	rendered := func() string { return metrics.Default().WritePrometheus() }
	counterLine := func(status string) string {
		return `gitferry_api_ratelimit_total{host="hook-test.invalid",status="` + status + `"} `
	}

	// 200 不打点
	rateLimitResponseHook(t.Context(), newReq(), &http.Response{StatusCode: 200, Header: http.Header{}}, 0, nil)
	assert.NotContains(t, rendered(), counterLine("200"))

	// 429 打点
	rateLimitResponseHook(t.Context(), newReq(), &http.Response{StatusCode: 429, Header: http.Header{}}, 0, nil)
	assert.Contains(t, rendered(), counterLine("429")+"1")

	// 403 + X-RateLimit-Remaining=0 打点
	h := http.Header{}
	h.Set("X-RateLimit-Remaining", "0")
	rateLimitResponseHook(t.Context(), newReq(), &http.Response{StatusCode: 403, Header: h}, 0, nil)
	assert.Contains(t, rendered(), counterLine("403")+"1")

	// 403 但额度未耗尽(如普通权限拒绝)不打点
	rateLimitResponseHook(t.Context(), newReq(), &http.Response{StatusCode: 403, Header: http.Header{}}, 0, nil)
	assert.NotContains(t, rendered(), counterLine("403")+"2")
}
