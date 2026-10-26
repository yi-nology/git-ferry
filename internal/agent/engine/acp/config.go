// Package acp 外部 AI coding agent 后端。
//
// 通过 **ACP(Agent Client Protocol)** 驱动外部成熟 coding 工具
// (MiniMax Code / Claude Code / Codex / Gemini CLI / OpenCode …),
// 让 git-ferry 的 AI 助手可以「不使用内部模型」,把回合交给外部 agent。
//
// 两种传输,共用同一套报文解析:
//
//   - acpx(AcpxEngine):acpx 是 ACP 的无头 CLI 客户端,一个命令面对 20+ 个 agent。
//     它把整段 ACP JSON-RPC 流打到 stdout ⇒ 解析代码与原生 transport 完全一致。
//     代价:权限决策交给 acpx 的档位,拿不到 session/request_permission。
//   - stdio(Engine):直接对 agent 子进程说 JSON-RPC,自己当 client。
//     能拿到 session/request_permission(可接进 git-ferry 的确认 UI),
//     也能把 git-ferry 自己的 /mcp 挂给 agent。
//
// **协议形状全部来自实测**(mcode acp / acpx 0.21.0),不是照抄文档:
// 见 wire.go 顶部与 updates.go 的注释。
package acp

import (
	"context"
	"time"
)

// 传输方式。
const (
	// TransportACPX 走 acpx CLI。
	TransportACPX = "acpx"
	// TransportStdio 直连 agent 子进程(原生 ACP)。
	TransportStdio = "stdio"
)

// 授权档(外部 agent 自身的行为边界)。
//
// 与 git-ferry 自己的工具权限档(tools.Policy)是两套东西:
// 前者管的是**外部 agent 在自己进程里**做什么,后者管的是 git-ferry 工具表。
const (
	// PermissionAsk 每次请求授权都交给用户确认(默认)。
	PermissionAsk = "ask"
	// PermissionApproveReads 只读类操作自动放行,写类仍要确认。
	PermissionApproveReads = "approve_reads"
	// PermissionApproveAll 全部放行(仅 admin 可切,且进审计)。
	PermissionApproveAll = "approve_all"
	// PermissionDeny 全拒(应急档)。
	PermissionDeny = "deny"
)

// 默认值。
const (
	// DefaultCwd 外部 agent 的默认工作目录(相对服务进程的工作目录)。
	DefaultCwd = "var/ai-workspace"
	// DefaultTimeout 单轮默认超时。
	DefaultTimeout = 10 * time.Minute
	// DefaultIdleTTL 原生 transport 的会话进程空闲回收时间。
	DefaultIdleTTL = 15 * time.Minute
	// defaultMaxProcs 原生 transport 同时存活的 agent 进程上限。
	defaultMaxProcs = 8
	// acpxBinary acpx 可执行文件名(在 PATH 里找)。
	acpxBinary = "acpx"
)

// MCPServer 挂给外部 agent 的 MCP 服务器。
//
// 形状对齐 ACP 规范(session/new 的 mcpServers):
// stdio 走 command/args/env;http/sse 走 url/headers。
// **headers 是数组 [{name,value}]**,不是对象 —— 这是规范原文的形状。
type MCPServer struct {
	// Name 人类可读标识(必填)。
	Name string `json:"name"`
	// Type 传输类型:stdio | http | sse。留空视为 stdio。
	Type string `json:"type,omitempty"`
	// Command stdio 传输的可执行文件绝对路径。
	Command string `json:"command,omitempty"`
	// Args stdio 传输的参数。
	Args []string `json:"args,omitempty"`
	// Env stdio 传输的环境变量。
	Env []EnvVar `json:"env,omitempty"`
	// URL http/sse 传输的地址。
	URL string `json:"url,omitempty"`
	// Headers http/sse 传输的请求头。
	Headers []HttpHeader `json:"headers,omitempty"`
}

// EnvVar 环境变量。
type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// HttpHeader HTTP 请求头。
type HttpHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Config 外部 coding agent 后端配置。
type Config struct {
	// Transport 传输方式:acpx | stdio。留空按 stdio。
	Transport string `json:"transport,omitempty"`

	// Binary acpx 可执行文件(默认在 PATH 里找 acpx)。
	Binary string `json:"binary,omitempty"`

	// Agent acpx 内置 agent 名(mcode / codex / claude / gemini / opencode …)。
	Agent string `json:"agent,omitempty"`
	// Command 自定义命令(原生 transport 直接执行它;acpx transport 作为 --agent 覆盖)。
	Command string `json:"command,omitempty"`
	// Args 追加参数。
	Args []string `json:"args,omitempty"`

	// Cwd 工作目录(沙箱边界)。相对路径按服务进程工作目录解析。
	Cwd string `json:"cwd,omitempty"`
	// Timeout 单轮超时。
	Timeout time.Duration `json:"timeout,omitempty"`
	// Permission 授权档。
	Permission string `json:"permission,omitempty"`

	// MCPServers 挂给 agent 的 MCP 服务器(原生 transport 生效)。
	MCPServers []MCPServer `json:"mcp_servers,omitempty"`

	// MaxProcs 原生 transport 同时存活的 agent 进程上限。
	MaxProcs int `json:"max_procs,omitempty"`
	// IdleTTL 会话进程空闲回收时间。
	IdleTTL time.Duration `json:"idle_ttl,omitempty"`

	// clientInfo 写进 initialize 的客户端标识。
	clientInfo clientInfo
}

// clientInfo initialize 里的客户端标识。
type clientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// normalize 补齐默认值。
func (c *Config) normalize() {
	if c.Transport == "" {
		c.Transport = TransportStdio
	}
	if c.Cwd == "" {
		c.Cwd = DefaultCwd
	}
	if c.Timeout <= 0 {
		c.Timeout = DefaultTimeout
	}
	if c.Permission == "" {
		c.Permission = PermissionAsk
	}
	if c.MaxProcs <= 0 {
		c.MaxProcs = defaultMaxProcs
	}
	if c.IdleTTL <= 0 {
		c.IdleTTL = DefaultIdleTTL
	}
	if c.Binary == "" {
		c.Binary = acpxBinary
	}
	if c.clientInfo.Name == "" {
		c.clientInfo.Name = "git-ferry"
	}
	if c.clientInfo.Version == "" {
		c.clientInfo.Version = "1.0.0"
	}
}

// Validate 校验配置并补默认值。
//
// **不静默降级**:命令缺失、agent 名为空这类问题在这里直接报错,
// 由调用方如实告诉用户「这个后端用不了」,而不是悄悄退回内置助手。
func (c *Config) Validate() error {
	c.normalize()
	switch c.Transport {
	case TransportACPX:
		if c.Agent == "" && c.Command == "" {
			return errConfig("acpx 后端需要 agent 名(如 mcode/codex/claude)或自定义 command")
		}
	case TransportStdio:
		if c.Command == "" {
			return errConfig("stdio 后端需要 command(ACP agent 的可执行命令,如 \"mcode acp\")")
		}
	default:
		return errConfig("未知 transport: " + c.Transport + "(可选 acpx|stdio)")
	}
	switch c.Permission {
	case PermissionAsk, PermissionApproveReads, PermissionApproveAll, PermissionDeny:
	default:
		return errConfig("未知 permission 档: " + c.Permission +
			"(可选 ask|approve_reads|approve_all|deny)")
	}
	return nil
}

// AuditFunc 外部 agent 行为的审计回调(可选)。
//
// **为什么需要它**:外部 agent 的工具调用发生在**它自己的进程里**
// (读写文件、跑 shell),git-ferry 既有的操作审计看不见它们;
// 而经 MCP 调回本服务的那部分则**已���**由 core 记录。
// 这里补的是「agent 在本机做了什么」这一段。
type AuditFunc func(ctx context.Context, action, tool, detail string)

// auditAction 审计动作名(固定值,便于检索)。
const (
	auditActionToolCall   = "external_agent.tool_call"
	auditActionPermission = "external_agent.permission"
)
