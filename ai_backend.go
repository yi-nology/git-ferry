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
	ext := cfg.External
	timeout := time.Duration(ext.TimeoutSeconds) * time.Second

	var (
		eng engine.Engine
		err error
	)
	switch cfg.Backend {
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
		servers, serr := mcpServersFor(&ext, shellCfg)
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
func externalAuditFunc(svc *corebridge.Service, ext *agent.ExternalConfig) acp.AuditFunc {
	name := ext.Agent
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

// mcpServersFor 组装最终挂给外部 agent 的 MCP 服务器清单。
//
// 清单 = (mcp_enabled ? 本服务自己的 /mcp : 无) + external.mcp_servers(额外的)。
func mcpServersFor(ext *agent.ExternalConfig, shellCfg *corebridge.ShellConfig) ([]acp.MCPServer, error) {
	var out []acp.MCPServer
	if ext.MCPEnabled {
		srv, err := mcpServerFor(ext, shellCfg)
		if err != nil {
			return nil, err
		}
		out = append(out, srv)
	}
	for i := range ext.MCPServers {
		srv, err := mcpServerFromConfig(&ext.MCPServers[i])
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
func mcpServerFor(ext *agent.ExternalConfig, shellCfg *corebridge.ShellConfig) (acp.MCPServer, error) {
	url := ext.MCPURL
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
	if ext.MCPAPIKeyEnv == "" {
		// 没配专用 key ⇒ 外部 agent 带的是别人的凭据或匿名,一律视为不可用。
		return srv, fmt.Errorf("ai.external.mcp_enabled 需要 ai.external.mcp_api_key_env " +
			"(挂给外部 agent 的 MCP 专用 key 的环境变量名;该 key 固定为只读角色)")
	}
	key := os.Getenv(ext.MCPAPIKeyEnv)
	if key == "" {
		return srv, fmt.Errorf("环境变量 %s 未设置(外部 agent 的 MCP 专用 key)", ext.MCPAPIKeyEnv)
	}
	srv.Headers = []acp.HttpHeader{{Name: "X-API-Key", Value: key}}
	// 注册成带角色的附加 key(固定只读);旧的同用途 key 立即失效。
	git_sync.SetAIMCPAPIKey(key)
	return srv, nil
}

// attachDelegator 装配「把任务委派给外部 coding agent」的执行器。
//
// 与后端选择**正交**:即便主后端是内置助手,也允许把编码任务派给外部 agent
// (ai_code_delegate 工具)。没配外部 agent 时不装配 ——
// 工具会如实返回「外部 coding agent 未配置」,而不是静默失败。
func attachDelegator(cfg *agent.Config, eng engine.Engine) {
	ext := cfg.External
	if ext.Agent == "" && ext.Command == "" {
		return
	}
	dr, err := acp.NewDelegateRunner(&acp.Config{
		Agent:      ext.Agent,
		Command:    ext.Command,
		Args:       ext.Args,
		Cwd:        ext.Cwd,
		Timeout:    time.Duration(ext.TimeoutSeconds) * time.Second,
		Permission: ext.Permission,
	})
	if err != nil {
		slog.Warn("委派执行器未装配(外部 agent 不可用)", "error", err)
		return
	}
	b, ok := eng.(engine.DelegatorBinder)
	if !ok {
		return
	}
	b.SetDelegator(dr)
	slog.Info("ai delegate ready", "agent", ext.Agent, "command", ext.Command)
}

// logAIBackend 记录后端装配结果(启动日志)。
func logAIBackend(cfg *agent.Config, eng engine.Engine) {
	if eng == nil {
		return
	}
	slog.Info("ai backend ready",
		"backend", eng.ID(),
		"label", eng.Label(),
		"agent", eng.ModelName(),
		"mcp_enabled", cfg.External.MCPEnabled,
	)
}
