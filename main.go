package main

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
	"github.com/yi-nology/git-sync-service/biz/handler/git_sync"
	"github.com/yi-nology/git-sync-service/sync"

	// Register all platform backends (GitHub, GitLab, Gitea, etc.)
	_ "github.com/yi-nology/git-platform-sdk/backends/all"
)

var syncSvc *sync.Service

// customRecovery 返回 JSON 格式的 panic 恢复中间件,
// 替代 Hertz 默认的 recovery.Recovery() (返回 HTML/text,前端无法解析)。
func customRecovery() app.HandlerFunc {
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

func main() {
	cfg, err := sync.LoadConfig("conf/config.yaml")
	if err != nil {
		slog.Error("load config failed", "error", err)
		os.Exit(1)
	}

	// Configure logger based on config
	var logLevel slog.Level
	switch cfg.Log.Level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	var logHandler slog.Handler
	if cfg.Log.Format == "text" {
		logHandler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: logLevel,
		})
	} else {
		logHandler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: logLevel,
		})
	}
	slog.SetDefault(slog.New(logHandler))

	syncSvc, err = sync.NewService(cfg)
	if err != nil {
		slog.Error("init sync service failed", "error", err)
		os.Exit(1)
	}

	if err := syncSvc.Start(); err != nil {
		slog.Error("start sync service failed", "error", err)
		os.Exit(1)
	}

	git_sync.SetSyncServiceGetter(func() *sync.Service {
		return syncSvc
	})

	h := server.Default(
		server.WithHostPorts(fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)),
		server.WithMaxRequestBodySize(cfg.Webhook.MaxBodySize),
	)

	// Recovery 中间件:handler panic 时返回 JSON 500 而非默认 HTML/text
	h.Use(customRecovery())
	// gzip 压缩:JSON 响应体积减 60-80%
	h.Use(gzip.Gzip(gzip.DefaultCompression))

	register(h)

	// Use oklog/run for structured concurrency with deterministic teardown.
	var g run.Group

	// Actor 1: HTTP server
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

	// Actor 2: OS signal handler (SIGINT / SIGTERM)
	g.Add(run.SignalHandler(context.Background(), syscall.SIGINT, syscall.SIGTERM))

	// Block until the first actor returns; all actors are then interrupted
	// and Run() returns only after every actor has exited.
	err = g.Run()

	// Clean up the sync service after the HTTP server has fully stopped.
	syncSvc.Stop()

	if err != nil && !errors.Is(err, run.ErrSignal) {
		slog.Error("run group exited with error", "error", err)
		os.Exit(1)
	}
}
