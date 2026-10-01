package main

import (
	"context"
	"log/slog"
	"sync"

	"github.com/yi-nology/git-ferry/biz/handler/git_sync"
	"github.com/yi-nology/git-ferry/biz/serve"
	"github.com/yi-nology/git-ferry/internal/agent"
	"github.com/yi-nology/git-ferry/internal/agent/memory"
	"github.com/yi-nology/git-ferry/internal/agent/tools"
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
	git_sync.SetAPIKeyRole(shellCfg.APIKeyRole)
	if shellCfg.GitServe != nil {
		git_sync.SetGitServe(shellCfg.GitServe.Enabled, shellCfg.GitServe.BasePath, shellCfg.GitServe.PublicRead)
	}
	if shellCfg.OIDC != nil && shellCfg.OIDC.Enabled {
		git_sync.SetOIDCConfig(&git_sync.OIDCConfig{
			Enabled:     shellCfg.OIDC.Enabled,
			Secret:      shellCfg.OIDC.Secret,
			Issuer:      shellCfg.OIDC.Issuer,
			Audience:    shellCfg.OIDC.Audience,
			RoleClaim:   shellCfg.OIDC.RoleClaim,
			UserClaim:   shellCfg.OIDC.UserClaim,
			DefaultRole: shellCfg.OIDC.DefaultRole,
		})
	}

	// AI 助手:yaml 默认 + data/ai-settings.json 界面覆盖;未启用 → Runner=nil,端点 501
	yamlAI, err := agent.LoadConfig("conf/config.yaml")
	if err != nil {
		serve.ExitOnFail("load ai config failed", err)
	}
	aiStore, err := agent.OpenSettings("data/ai-settings.json")
	if err != nil {
		serve.ExitOnFail("open ai settings failed", err)
	}
	cur := aiStore.Get()
	aiSettings := agent.MergeSettings(yamlAI, &cur)

	var aiRunnerMu sync.RWMutex
	var aiRunner *agent.Runner

	buildRunner := func(st *agent.Settings) (*agent.Runner, error) {
		if st == nil || !st.Enabled {
			return nil, nil
		}
		cfg := st.ToConfig()
		apiKey := st.APIKey
		if apiKey == "" {
			apiKey = agent.APIKeyFromEnv()
		}
		if err := cfg.Validate(apiKey); err != nil {
			return nil, err
		}
		memPath := cfg.MemoryPath
		if memPath == "" {
			memPath = "data/ai-memory.json"
		}
		memStore, err := memory.Open(memPath)
		if err != nil {
			return nil, err
		}
		cfg.MemoryManifest = agent.MemoryManifest(memStore, 10)
		r, err := agent.NewRunner(cfg, apiKey, syncSvc)
		if err != nil {
			return nil, err
		}
		r.SetMemory(agent.NewMemBridge(memStore), agent.MemoryManifest(memStore, 10))
		r.SetPolicy(tools.NewPolicy())
		agent.SetPersistDir(memPath[:max(0, len(memPath)-len("/ai-memory.json"))] + "/ai-tool-output")
		slog.Info("ai assistant ready", "model", cfg.Model, "base_url", cfg.BaseURL, "memory", memPath)
		return r, nil
	}

	// 启动时按合并结果构建(启用失败不退出:设置页可改后热生效)
	if r, err := buildRunner(&aiSettings); err != nil {
		slog.Warn("ai disabled at boot, fix via settings page", "error", err)
	} else {
		aiRunnerMu.Lock()
		aiRunner = r
		aiRunnerMu.Unlock()
		if r != nil {
			slog.Info("ai tools: policy=default, persist=on")
		}
	}

	git_sync.SetAgentRunner(func() *agent.Runner {
		aiRunnerMu.RLock()
		defer aiRunnerMu.RUnlock()
		return aiRunner
	})
	git_sync.SetAISettingsStore(func() *agent.SettingsStore { return aiStore })
	git_sync.SetAIRebuildRunner(func(st *agent.Settings) error {
		r, err := buildRunner(st)
		if err != nil {
			return err
		}
		aiRunnerMu.Lock()
		aiRunner = r
		aiRunnerMu.Unlock()
		return nil
	})

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
