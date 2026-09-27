package git_sync

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/yi-nology/git-ferry/internal/metrics"
)

// MetricsMiddleware 记录 HTTP 请求量(方法/路径/状态码)。
func MetricsMiddleware() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		c.Next(ctx)
		metrics.ObserveHTTP(string(c.Method()),
			metrics.LowCardinalityPath(string(c.Path())), c.Response.StatusCode())
	}
}
