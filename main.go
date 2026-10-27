package main

import (
	"context"
	"sync"

	"github.com/oekit/logx"
	"github.com/yi-nology/git-ferry/biz/handler/git_sync"
	"github.com/yi-nology/git-ferry/biz/serve"
	"github.com/yi-nology/git-ferry/internal/agent"
	"github.com/yi-nology/git-ferry/internal/agent/memory"
	"github.com/yi-nology/git-ferry/internal/agent/tools"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/notify"
	"github.com/yi-nology/git-ferry/internal/runwatch"

	// Register all platform backends (GitHub, GitLab, Gitea, etc.)

	_ "github.com/yi-nology/go-git-platform/backends/all"
)

// logger 是本包的日志出口(经 logx 门面)。
//
// **用 Delegating 而不是 Default()**:包级 var 在 init 时求值,
// 直接捕获 Default() 会永远绑在 init 那一刻的默认 logger 上,
// 消费方随后的 SetDefault 对它无效;Delegating 是**调用时**解析,
// 自动跟随 main() 里装好的后端。
var logger = logx.Delegating("git-ferry")

func main() {
	shellCfg, err := corebridge.LoadShellConfig("conf/config.yaml")
	if err != nil {
		serve.ExitOnFail("load config failed", err)
	}

	serve.SetupLogger(shellCfg.Log.Level, shellCfg.Log.Format)

	// provider 限流指标钩子与 force-push 审批器经 Option 传入，
	// 随 core 构造一次性生效，不再依赖"先于 NewService 调用"的时序约定。
	syncSvc, err := corebridge.NewService(shellCfg.Config,
		corebridge.WithProviderHooks(git_sync.ProviderHooks()),
	)
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
	// force-push 审批存储随 core NewService 建立（<backup_dir>/force-push-approvals.json），
	// 执行器分歧保护默认回调该存储；HTTP 端点见 forcepush_approval.go。
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
	// 注入 yaml 段:设置页要显示**生效**配置(设置文件只是覆盖层)
	git_sync.SetAIYamlConfig(yamlAI)

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
		// 执行后端:内置 eino 助手 / 外部 coding agent(acpx / 原生 ACP)。
		// 后端是**可插拔**的,门面(Runner)不变。
		eng, err := buildAIEngine(cfg, apiKey, syncSvc, syncSvc, shellCfg)
		if err != nil {
			return nil, err
		}
		// 委派执行器:即便主后端是内置助手,也允许把编码任务派给外部 agent。
		attachDelegator(cfg, eng)
		r := agent.NewRunner(eng, cfg.MaxConcurrentChats)
		r.SetMemory(agent.NewMemBridge(memStore), agent.MemoryManifest(memStore, 10))
		r.SetPolicy(tools.NewPolicy())
		agent.SetPersistDir(memPath[:max(0, len(memPath)-len("/ai-memory.json"))] + "/ai-tool-output")
		logAIBackend(cfg, eng)
		return r, nil
	}

	// 启动时按合并结果构建(启用失败不退出:设置页可改后热生效)
	if r, err := buildRunner(&aiSettings); err != nil {
		logger.Warn("ai disabled at boot, fix via settings page", "error", err)
	} else {
		aiRunnerMu.Lock()
		aiRunner = r
		aiRunnerMu.Unlock()
		if r != nil {
			logger.Info("ai tools: policy=default, persist=on")
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
		old := aiRunner
		aiRunner = r
		aiRunnerMu.Unlock()
		// **关掉旧后端**:外部后端每个会话对应一个长驻 agent 子进程,
		// 换后端不关旧的话,机器上会攒下一堆孤儿进程。
		if old != nil {
			if cerr := old.Close(); cerr != nil {
				logger.Warn("close previous ai backend failed", "error", cerr)
			}
		}
		return nil
	})

	// 同步策略模板库已随 core NewService 打开（cfg.Templates.Path，默认 data/templates.json）

	// 通知矩阵 + 运行观察(失败补偿)
	// notify 段在配置中缺省时 shellCfg.Notify 为 nil,取字段前先判空
	notifier := notify.New(shellCfg.Notify)
	var hbCfg *notify.HeartbeatConfig
	if shellCfg.Notify != nil {
		hbCfg = shellCfg.Notify.Heartbeat
	}
	hb := notify.NewHeartbeat(hbCfg)
	watchCfg := runwatch.Config{}
	if shellCfg.RunWatch != nil {
		watchCfg = runwatch.Config{
			Mode:            shellCfg.RunWatch.Mode,
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
		// 退出前关掉 AI 后端(关外部 agent 子进程)
		aiRunnerMu.Lock()
		runner := aiRunner
		aiRunnerMu.Unlock()
		if runner != nil {
			if err := runner.Close(); err != nil {
				logger.Warn("close ai backend failed", "error", err)
			}
		}
		syncSvc.Stop()
		// webhook 限流器带后台空闲回收 goroutine,退出前关闭
		git_sync.CloseWebhookRateLimiter()
	}
	if err := serve.Run(h, cleanup); err != nil {
		serve.ExitOnFail("server exited with error", err)
	}
}
