package main

import (
	"github.com/yi-nology/git-sync-service/biz/handler/git_sync"
	"github.com/yi-nology/git-sync-service/biz/serve"
	"github.com/yi-nology/git-sync-service/internal/corebridge"

	// Register all platform backends (GitHub, GitLab, Gitea, etc.)
	_ "github.com/yi-nology/git-platform-sdk/backends/all"
)

func main() {
	shellCfg, err := corebridge.LoadShellConfig("conf/config.yaml")
	if err != nil {
		serve.ExitOnFail("load config failed", err)
	}

	serve.SetupLogger(shellCfg.Log.Level, shellCfg.Log.Format)

	syncSvc, err := corebridge.NewService(shellCfg.Config)
	if err != nil {
		serve.ExitOnFail("init sync service failed", err)
	}

	if err := syncSvc.Start(); err != nil {
		serve.ExitOnFail("start sync service failed", err)
	}

	git_sync.SetSyncServiceGetter(func() *corebridge.Service {
		return syncSvc
	})
	git_sync.SetAPIKey(shellCfg.APIKey)

	h := serve.New(serve.Config{
		Host:        shellCfg.Server.Host,
		Port:        shellCfg.Server.Port,
		MaxBodySize: shellCfg.Webhook.MaxBodySize,
	})

	if err := serve.Run(h, syncSvc.Stop); err != nil {
		serve.ExitOnFail("run group exited with error", err)
	}
}
