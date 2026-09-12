package main

import (
	"log/slog"
	"os"

	"github.com/yi-nology/git-sync-core"
	"github.com/yi-nology/git-sync-service/biz/handler/git_sync"
	"github.com/yi-nology/git-sync-service/biz/serve"

	// Register all platform backends (GitHub, GitLab, Gitea, etc.)
	_ "github.com/yi-nology/git-platform-sdk/backends/all"
)

func main() {
	cfg, err := sync.LoadConfig("conf/config.yaml")
	if err != nil {
		slog.Error("load config failed", "error", err)
		os.Exit(1)
	}

	serve.SetupLogger(cfg.Log.Level, cfg.Log.Format)

	syncSvc, err := sync.NewService(cfg)
	if err != nil {
		serve.ExitOnFail("init sync service failed", err)
	}

	if err := syncSvc.Start(); err != nil {
		serve.ExitOnFail("start sync service failed", err)
	}

	git_sync.SetSyncServiceGetter(func() *sync.Service {
		return syncSvc
	})

	h := serve.New(serve.Config{
		Host:        cfg.Server.Host,
		Port:        cfg.Server.Port,
		MaxBodySize: cfg.Webhook.MaxBodySize,
	})

	if err := serve.Run(h, syncSvc.Stop); err != nil {
		serve.ExitOnFail("run group exited with error", err)
	}
}
