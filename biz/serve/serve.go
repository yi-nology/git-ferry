// Package serve 提供公网/内网壳共用的 HTTP 启动能力：
// JSON recovery、gzip、完整路由注册、oekit bootstrap 驱动的优雅退出。
// 壳层只负责：配置、core.Service 初始化、鉴权 Provider、生命周期清理。
package serve

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/hertz-contrib/gzip"
	"github.com/oekit/bootstrap"
	"github.com/oekit/core/chain"
	hertzx "github.com/oekit/hertz"
	"github.com/oekit/hertz/chainmw"
	"github.com/yi-nology/git-ferry-core/pkg/proclog"
	"github.com/yi-nology/git-ferry/biz/router"
)

// serviceName 是 bootstrap 生命周期与日志里的服务标识。
const serviceName = "git-ferry"

// Config 是 HTTP 服务监听相关配置（与 core.Config.Server/Webhook 对齐字段）。
type Config struct {
	Host        string
	Port        int
	MaxBodySize int
}

// Recovery 返回 JSON 格式的 panic 恢复中间件，
// 替代 Hertz 默认 recovery.Recovery()（HTML/text，前端无法解析）。
//
// 用 oekit core/chain + hertz/chainmw：panic 由 chain.Recovery 收编为
// *chain.PanicError（带原始 payload 与栈），错误出口经 chainmw.WithOnError
// 完全接管——chainmw 的默认出口是 middleware.Fail 的统一响应格式，
// 本仓要的是自己的 JSON 体。
func Recovery() app.HandlerFunc {
	return chainmw.Wrap(
		[]chain.Middleware{
			chain.Recovery(chain.WithPanicReporter(func(_ context.Context, req any, pv chain.PanicError) {
				method, path := "", ""
				if c, ok := req.(*app.RequestContext); ok {
					method, path = string(c.Method()), string(c.Path())
				}
				slog.Error("handler panic recovered",
					"panic", fmt.Sprintf("%v", pv.Value),
					"stack", pv.Stack,
					"method", method,
					"path", path,
				)
			})),
		},
		func(ctx context.Context, c *app.RequestContext) { c.Next(ctx) },
		chainmw.WithOnError(func(_ context.Context, c *app.RequestContext, _ error) {
			c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
			c.Abort()
		}),
	)
}

// Register 注册全部路由（IDL 生成 + 定制探活/webhook）。
func Register(r *server.Hertz) {
	router.GeneratedRegister(r)
	router.CustomizedRegister(r)
}

// New 构建带中间件与完整路由的 Hertz 实例。
// extra 会追加在 recovery/gzip 之后、路由之前。
func New(cfg Config, extra ...app.HandlerFunc) *server.Hertz {
	maxBody := cfg.MaxBodySize
	if maxBody <= 0 {
		maxBody = 10 << 20
	}
	host := cfg.Host
	if host == "" {
		host = "0.0.0.0"
	}
	port := cfg.Port
	if port <= 0 {
		port = 8890
	}

	h := server.Default(
		server.WithHostPorts(fmt.Sprintf("%s:%d", host, port)),
		server.WithMaxRequestBodySize(maxBody),
	)
	h.Use(Recovery())
	// gzip 排除 AI SSE 路径:压缩层缓冲与流式 flush 语义相性差,
	// EventSource/流式 fetch 需要事件即时可见
	h.Use(gzip.Gzip(gzip.DefaultCompression,
		gzip.WithExcludedPathRegexes([]string{`^/api/v1/ai/.*`}),
	))
	for _, mw := range extra {
		if mw != nil {
			h.Use(mw)
		}
	}
	Register(h)
	return h
}

// Run 启动 HTTP 服务并阻塞至 SIGINT/SIGTERM,随后优雅关闭并执行 cleanup。
// 生命周期交给 oekit bootstrap:资源按注册逆序关闭——cleanup 先注册、后执行,
// 保证 HTTP 排空完成后才释放 core.Service 等依赖(与此前 run.Group 的顺序一致)。
func Run(h *server.Hertz, cleanup func()) error {
	b, err := bootstrap.New(
		bootstrap.WithServiceName(serviceName),
		bootstrap.WithShutdownTimeout(10*time.Second),
	)
	if err != nil {
		return err
	}
	return b.Run(func(app *bootstrap.App) error {
		if cleanup != nil {
			app.AddResource("shell-cleanup", func(context.Context) error {
				cleanup()
				return nil
			})
		}
		hertzx.RunServer(app, "http", h)
		<-app.Context().Done()
		return nil
	})
}

// ExitOnFail 打日志并退出进程，供壳层 main 使用。
func ExitOnFail(msg string, err error) {
	proclog.ExitOnFail(msg, err)
}

// SetupLogger 按 level/format 配置默认 slog；实现在 core pkg/proclog（两壳共用）。
func SetupLogger(level, format string) {
	proclog.Setup(level, format)
}
