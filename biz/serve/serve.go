// Package serve 提供公网/内网壳共用的 HTTP 启动能力：
// JSON recovery、gzip、完整路由注册、信号驱动的优雅退出。
// 壳层只负责：配置、core.Service 初始化、鉴权 Provider、生命周期清理。
package serve

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/hertz-contrib/gzip"
	"github.com/oklog/run"
	"github.com/yi-nology/git-ferry/biz/router"
)

// Config 是 HTTP 服务监听相关配置（与 core.Config.Server/Webhook 对齐字段）。
type Config struct {
	Host        string
	Port        int
	MaxBodySize int
}

// Recovery 返回 JSON 格式的 panic 恢复中间件，
// 替代 Hertz 默认 recovery.Recovery()（HTML/text，前端无法解析）。
func Recovery() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("handler panic recovered",
					"panic", fmt.Sprintf("%v", r),
					"stack", string(debug.Stack()),
					"method", string(c.Method()),
					"path", string(c.Path()),
				)
				c.JSON(http.StatusInternalServerError, map[string]string{
					"error": "internal server error",
				})
				c.Abort()
			}
		}()
		c.Next(ctx)
	}
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

// Run 启动 HTTP 服务并阻塞至 SIGINT/SIGTERM，随后优雅关闭并执行 cleanup。
func Run(h *server.Hertz, cleanup func()) error {
	var g run.Group

	g.Add(func() error {
		h.Spin()
		return nil
	}, func(error) {
		slog.Info("shutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := h.Shutdown(ctx); err != nil {
			slog.Error("server shutdown error", "error", err)
		}
		slog.Info("server stopped")
	})

	g.Add(run.SignalHandler(context.Background(), syscall.SIGINT, syscall.SIGTERM))

	err := g.Run()
	if cleanup != nil {
		cleanup()
	}
	if err != nil && !errors.Is(err, run.ErrSignal) {
		return err
	}
	return nil
}

// ExitOnFail 打日志并退出进程，供壳层 main 使用。
func ExitOnFail(msg string, err error) {
	slog.Error(msg, "error", err)
	os.Exit(1)
}

// SetupLogger 按 level/format 配置默认 slog（debug|info|warn|error × json|text）。
func SetupLogger(level, format string) {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	var h slog.Handler
	if format == "text" {
		h = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	} else {
		h = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	}
	slog.SetDefault(slog.New(h))
}
