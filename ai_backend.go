package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/yi-nology/git-ferry/biz/handler/git_sync"
	"github.com/yi-nology/git-ferry/internal/agent"
	"github.com/yi-nology/git-ferry/internal/agent/engine"
	"github.com/yi-nology/git-ferry/internal/agent/engine/acp"
	"github.com/yi-nology/git-ferry/internal/agent/tools"
	"github.com/yi-nology/git-ferry/internal/corebridge"
)

// buildAIEngine 按配置装配 AI 执行后端。
//
// 三种后端:
//   - internal:内置 eino 助手(OpenAI 兼容端点,工具在本进程执行);
//   - acpx:外部 coding agent(acpx CLI 驱动);
//   - stdio:外部 coding agent(原生 ACP,自己当 client,可挂 git-ferry 的 /mcp)。
//
// **装配在 main 这层做** —— internal/agent 不 import engine/acp,
// 免得底层后端反过来依赖上层包。
func buildAIEngine(cfg *agent.Config, apiKey string, svc tools.SyncService,
	coreSvc *corebridge.Service, shellCfg *corebridge.ShellConfig) (engine.Engine, error) {
	// **装配对象是生效的那个 profile**,不是旧的单字段 ——
	// profile 模式下命令/MCP 挂载都写在 profile 里,查单字段会装出一个空后端。
	ext, transport := cfg.EffectiveExternal()
	timeout := time.Duration(ext.TimeoutSeconds) * time.Second

	var (
		eng engine.Engine
		err error
	)
	switch transport {
	case agent.BackendACPX:
		eng, err = acp.NewAcpxEngine(&acp.Config{
			Agent:      ext.Agent,
			Command:    ext.Command,
			Args:       ext.Args,
			Cwd:        ext.Cwd,
			Timeout:    timeout,
			Permission: ext.Permission,
		})

	case agent.BackendStdio:
		c := acp.Config{
			Command:    ext.Command,
			Args:       ext.Args,
			Cwd:        ext.Cwd,
			Timeout:    timeout,
			Permission: ext.Permission,
		}
		servers, serr := mcpServersFor(ext.MCPEnabled, ext.MCPURL, ext.MCPAPIKeyEnv, ext.MCPServers, shellCfg)
		if serr != nil {
			return nil, serr
		}
		c.MCPServers = servers
		eng, err = acp.NewEngine(&c)

	default:
		return agent.NewEinoEngine(cfg, apiKey, svc)
	}
	if err != nil {
		return nil, err
	}
	// 审计:外部 agent 在**本机**的动作(读写文件、跑 shell)不进 core 的操作日志,
	// 这里补一条,让「agent 做了什么」可查。
	if a, ok := eng.(auditable); ok {
		a.SetAudit(externalAuditFunc(coreSvc, &ext))
	}
	return eng, nil
}

// Auditable 支持注入审计出口的后端(外部 agent 类后端)。
type auditable interface{ SetAudit(f acp.AuditFunc) }

// externalAuditFunc 把外部 agent 的动作记进操作日志。
func externalAuditFunc(svc *corebridge.Service, ext *agent.ExternalAgentConfig) acp.AuditFunc {
	name := ext.Name
	if name == "" {
		name = ext.Agent
	}
	if name == "" {
		name = ext.Command
	}
	return func(ctx context.Context, action, tool, detail string) {
		entry := &corebridge.OperationLog{
			Action:       action,
			ResourceType: "ExternalAgent",
			ResourceKey:  tool,
			Resource:     tool,
			Actor:        "acp:" + name,
			Status:       corebridge.StatusSuccess,
			Detail:       detail,
		}
		if err := svc.RecordOperation(ctx, entry); err != nil {
			slog.Warn("外部 agent 审计写入失败", "error", err, "action", action, "tool", tool)
		}
	}
}

// acpConfigFor 由 profile 生成 acp 后端配置。
//
// transport 由 ProfileTransportOf 解析(见 agent 包的说明:
// profile 显式写的 > 有 command ⇒ stdio > 有 agent 名 ⇒ acpx > 跟随 ai.backend)。
func acpConfigFor(p *agent.ExternalAgentConfig, permission, backend string) acp.Config {
	return acp.Config{
		Transport:  agent.ProfileTransportOf(backend, p),
		Agent:      p.Agent,
		Command:    p.Command,
		Args:       p.Args,
		Cwd:        p.Cwd,
		Timeout:    time.Duration(p.TimeoutSeconds) * time.Second,
		Permission: permission,
	}
}

// mcpServersFor 组装最终挂给外部 agent 的 MCP 服务器清单。
//
// 清单 = (mcp_enabled ? 本服务自己的 /mcp : 无) + external.mcp_servers(额外的)。
func mcpServersFor(enabled bool, url, apiKeyEnv string,
	extra []agent.MCPServerConfig, shellCfg *corebridge.ShellConfig) ([]acp.MCPServer, error) {
	var out []acp.MCPServer
	if enabled {
		srv, err := mcpServerFor(url, apiKeyEnv, shellCfg)
		if err != nil {
			return nil, err
		}
		out = append(out, srv)
	}
	for i := range extra {
		srv, err := mcpServerFromConfig(&extra[i])
		if err != nil {
			return nil, err
		}
		out = append(out, srv)
	}
	return out, nil
}

// mcpServerFromConfig 把一条额外 MCP 服务器配置转成挂载项。
//
// 密钥从**环境变量**取(配置文件里只放变量名)—— 与本仓「密钥不进 yaml」的约定一致。
func mcpServerFromConfig(c *agent.MCPServerConfig) (acp.MCPServer, error) {
	if strings.TrimSpace(c.Name) == "" {
		return acp.MCPServer{}, errors.New("MCP 服务器缺 name")
	}
	srv := acp.MCPServer{
		Name:    c.Name,
		Type:    c.Type,
		URL:     c.URL,
		Command: c.Command,
		Args:    c.Args,
	}
	for _, e := range c.Env {
		srv.Env = append(srv.Env, acp.EnvVar{Name: e.Name, Value: e.Value})
	}
	if c.APIKeyEnv != "" {
		key := os.Getenv(c.APIKeyEnv)
		if key == "" {
			return acp.MCPServer{}, fmt.Errorf("环境变量 %s 未设置(MCP 服务器 %s 的认证头)", c.APIKeyEnv, c.Name)
		}
		header := c.APIKeyHeader
		if header == "" {
			header = "Authorization"
		}
		srv.Headers = append(srv.Headers, acp.HttpHeader{Name: header, Value: key})
	}
	return srv, nil
}

// mcpServerFor 组装挂给外部 agent 的 MCP 服务器(指向 git-ferry 自己的 /mcp)。
//
// 外部 agent 由此获得 41 个领域工具(同步/镜像/运维),不必重写一套。
//
// **专用 key 是只读角色**:MCP 路径上危险工具不弹确认,只过角色闸门。
// key 从环境变量取(不进 yaml、不进设置文件)。
func mcpServerFor(mcpURL, apiKeyEnv string, shellCfg *corebridge.ShellConfig) (acp.MCPServer, error) {
	url := mcpURL
	if url == "" {
		port := 0
		if shellCfg != nil {
			port = shellCfg.Server.Port
		}
		if port <= 0 {
			port = 8890
		}
		url = fmt.Sprintf("http://127.0.0.1:%d/mcp", port)
	}
	srv := acp.MCPServer{Name: "gitferry", Type: "http", URL: url}
	if apiKeyEnv == "" {
		// 没配专用 key ⇒ 外部 agent 带的是别人的凭据或匿名,一律视为不可用。
		return srv, fmt.Errorf("ai.external.mcp_enabled 需要 ai.external.mcp_api_key_env " +
			"(挂给外部 agent 的 MCP 专用 key 的环境变量名;该 key 固定为只读角色)")
	}
	key := os.Getenv(apiKeyEnv)
	if key == "" {
		return srv, fmt.Errorf("环境变量 %s 未设置(外部 agent 的 MCP 专用 key)", apiKeyEnv)
	}
	srv.Headers = []acp.HttpHeader{{Name: "X-API-Key", Value: key}}
	// 注册成带角色的附加 key(固定只读);旧的同用途 key 立即失效。
	git_sync.SetAIMCPAPIKey(key)
	return srv, nil
}

// delegatePermissionFor 委派用的**一次性**授权档。
//
// 一次性执行弹不出授权卡 ⇒ `ask` 在委派场景没有对应物。
// 映射成 approve_reads(只读自动放行),并在回执里明说 —— **不静默降级**。
//
// 注:这个映射是必须的,不是可选:acpx 是非交互通道,
// `ask` 会被校验直接拒掉 ⇒ 不映射的话,默认配置下**委派执行器根本装不上**
// (表现为 ai_code_delegate 报「未配置」,像没配过一样)。
func delegatePermissionFor(p string) string {
	if p == "" || p == acp.PermissionAsk {
		return acp.PermissionApproveReads
	}
	return p
}

// attachDelegator 装配「把任务委派给外部 coding agent」的执行器。
//
// 与后端选择**正交**:即便主后端是内置助手,也允许把编码任务派给外部 agent
// (ai_code_delegate 工具)。没配外部 agent 时不装配 ——
// 工具会如实返回「外部 coding agent 未配置」,而不是静默失败。
//
// **支持多个 profile**:委派时按名字选(命中 profile 用它的全部参数;
// 没命中就当 acpx 内置 agent 名用,保持旧行为)。
func attachDelegator(cfg *agent.Config, eng engine.Engine) {
	ext := cfg.External
	if len(ext.Agents) == 0 && ext.Agent == "" && ext.Command == "" {
		return
	}
	base := cfg.ResolveProfile(ext.Current)
	baseCfg := acpConfigFor(&base, delegatePermissionFor(base.Permission), cfg.Backend)
	dr, err := acp.NewDelegateRunner(&baseCfg)
	if err != nil {
		slog.Warn("委派执行器未装配(外部 agent 不可用)", "error", err)
		return
	}
	dr.WithResolver(func(name string) (acp.Config, bool) {
		p, ok := cfg.ResolveProfileByName(name)
		if !ok {
			return acp.Config{}, false
		}
		return acpConfigFor(&p, delegatePermissionFor(p.Permission), cfg.Backend), true
	})
	b, ok := eng.(engine.DelegatorBinder)
	if !ok {
		return
	}
	b.SetDelegator(dr)
	slog.Info("ai delegate ready", "agent", base.Agent, "command", base.Command,
		"profiles", cfg.ExternalProfileNames())
}

// logAIBackend 记录后端装配结果(启动日志)。
func logAIBackend(cfg *agent.Config, eng engine.Engine) {
	if eng == nil {
		return
	}
	ext, transport := cfg.EffectiveExternal()
	slog.Info("ai backend ready",
		"backend", eng.ID(),
		"label", eng.Label(),
		"agent", eng.ModelName(),
		"transport", transport,
		"profile", ext.Name,
		"mcp_enabled", ext.MCPEnabled,
	)
}
