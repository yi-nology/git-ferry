package git_sync

import (
	"context"
	"net/http"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
)

var startTime = time.Now()

// HealthCheck returns the health status of the service and its dependencies.
// 公开端点:不应因内部状态 panic(GetSyncService 可能返回 nil,如启动时序问题)。
func HealthCheck(ctx context.Context, c *app.RequestContext) {
	svc := GetSyncService()
	if svc == nil {
		c.JSON(http.StatusServiceUnavailable, map[string]any{
			"status": map[string]string{"service": "not initialized"},
		})
		return
	}
	status := svc.HealthCheck()

	httpStatus := http.StatusOK
	for _, v := range status {
		// Accept "ok" and "not configured" as healthy states
		if v != "ok" && v != "not configured" {
			httpStatus = http.StatusServiceUnavailable
			break
		}
	}

	c.JSON(httpStatus, map[string]any{
		"status": status,
	})
}
