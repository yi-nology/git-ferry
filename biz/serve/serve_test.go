package serve

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

// TestRecovery_PanicBecomesJSON500 handler panic 必须被收编成 JSON 500
// （前端要解析 JSON，不是 Hertz 默认的 HTML/text），且不影响后续请求。
func TestRecovery_PanicBecomesJSON500(t *testing.T) {
	h := server.New(server.WithHostPorts("127.0.0.1:0"))
	h.Use(Recovery())
	h.GET("/boom", func(ctx context.Context, c *app.RequestContext) {
		panic("kaboom")
	})
	h.GET("/ok", func(ctx context.Context, c *app.RequestContext) {
		c.String(http.StatusOK, "fine")
	})

	w := ut.PerformRequest(h.Engine, consts.MethodGet, "/boom", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("code=%d want 500", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"error"`) || !strings.Contains(body, "internal server error") {
		t.Fatalf("body=%q want JSON error envelope", body)
	}
	// 响应体应是 JSON 而非 Hertz 默认的 HTML
	if strings.Contains(strings.ToLower(body), "<html") {
		t.Fatalf("body=%q must not be HTML", body)
	}

	// recovery 之后服务仍可用(中间件没有把链打断死)
	w2 := ut.PerformRequest(h.Engine, consts.MethodGet, "/ok", nil)
	if w2.Code != http.StatusOK || w2.Body.String() != "fine" {
		t.Fatalf("after panic: code=%d body=%q", w2.Code, w2.Body.String())
	}
}
