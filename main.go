package main

import (
	"context"
	"log/slog"

	"github.com/yi-nology/git-ferry/biz/handler/git_sync"
	"github.com/yi-nology/git-ferry/biz/serve"
	"github.com/yi-nology/git-ferry/internal/agent"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/notify"
	"github.com/yi-nology/git-ferry/internal/runwatch"
	"github.com/yi-nology/git-ferry/internal/tpl"

	// Register all platform backends (GitHub, GitLab, Gitea, etc.)
	_ "github.com/yi-nology/go-git-platform/backends/all"
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

	// 同步策略模板库(文件型,零 DB 迁移)
	tplStore, err := tpl.Open("data/templates.json")
	if err != nil {
		serve.ExitOnFail("open template store failed", err)
	}
	git_sync.SetTplStore(tplStore)

	// 通知矩阵 + 运行观察(失败补偿)
	notifier := notify.New(shellCfg.Notify)
	hb := notify.NewHeartbeat(shellCfg.Notify.Heartbeat)
	watchCfg := runwatch.Config{}
	if shellCfg.RunWatch != nil {
		watchCfg = runwatch.Config{
			IntervalSeconds: shellCfg.RunWatch.IntervalSeconds,
			HistoryLimit:    shellCfg.RunWatch.HistoryLimit,
			Retry: runwatch.RetryConfig{
				MaxAutoRetries:  shellCfg.RunWatch.Retry.MaxAutoRetries,
				CooldownMinutes: shellCfg.RunWatch.Retry.CooldownMinutes,
			},
		}
	}
	watchCtx, watchCancel := context.WithCancel(context.Background())
	defer watchCancel()
	watcher := runwatch.New(syncSvc, notifier, hb, watchCfg)
	watcher.Start(watchCtx)
	if hb != nil {
		hb.Start(watchCtx)
	}

	h := serve.New(serve.Config{
		Host:        shellCfg.Server.Host,
		Port:        shellCfg.Server.Port,
		MaxBodySize: shellCfg.Webhook.MaxBodySize,
	}, git_sync.MetricsMiddleware())

	cleanup := func() {
		watchCancel()
		syncSvc.Stop()
	}
	if err := serve.Run(h, cleanup); err != nil {
		serve.ExitOnFail("run group exited with error", err)
	}
}
