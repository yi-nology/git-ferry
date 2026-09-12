package git_sync

import (
	"context"
	"net/http"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
)

var startTime = time.Now()

// HealthCheck returns the health status of the service and its dependencies.
func HealthCheck(ctx context.Context, c *app.RequestContext) {
	status := GetSyncService().HealthCheck()

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
