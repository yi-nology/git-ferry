package main

import (
	"log/slog"

	"github.com/yi-nology/git-sync-service/biz/handler/git_sync"
	"github.com/yi-nology/git-sync-service/biz/serve"
	"github.com/yi-nology/git-sync-service/internal/agent"
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

	// AI 助手:ai 段未启用 → 不构建 Runner,端点 501 降级
	aiCfg, err := agent.LoadConfig("conf/config.yaml")
	if err != nil {
		serve.ExitOnFail("load ai config failed", err)
	}
	var aiRunner *agent.Runner
	if aiCfg.Enabled {
		if err := aiCfg.Validate(agent.APIKeyFromEnv()); err != nil {
			serve.ExitOnFail("ai config invalid", err)
		}
		aiRunner, err = agent.NewRunner(aiCfg, agent.APIKeyFromEnv(), syncSvc)
		if err != nil {
			serve.ExitOnFail("init ai runner failed", err)
		}
		slog.Info("ai assistant enabled", "model", aiCfg.Model, "base_url", aiCfg.BaseURL)
	}
	git_sync.SetAgentRunner(func() *agent.Runner { return aiRunner })

	h := serve.New(serve.Config{
		Host:        shellCfg.Server.Host,
		Port:        shellCfg.Server.Port,
		MaxBodySize: shellCfg.Webhook.MaxBodySize,
	})

	if err := serve.Run(h, syncSvc.Stop); err != nil {
		serve.ExitOnFail("run group exited with error", err)
	}
}
