// AI 助手配置/对话 DTO(IDL 生成,与 API 字段对齐)。

struct AIConfigRequest {
    1: bool enabled (api.json="enabled")
    2: string baseUrl (api.json="base_url")
    3: string model (api.json="model")
    4: double temperature (api.json="temperature")
    5: i32 maxTokens (api.json="max_tokens")
    6: i32 timeoutSeconds (api.json="timeout_seconds")
    7: i32 maxConcurrentChats (api.json="max_concurrent_chats")
    8: optional string apiKey (api.json="api_key")
    // backend 执行后端:internal|acpx|stdio
    9: optional string backend (api.json="backend")
    // external 外部 coding agent 配置(backend=acpx|stdio 时生效)
    10: optional AIExternalConfig external (api.json="external")
}

// AIExternalConfig 外部 coding agent 配置。
struct AIExternalConfig {
    // agent acpx 内置 agent 名(mcode/codex/claude/gemini/opencode…)
    1: string agent (api.json="agent")
    // command 自定义命令(原生 ACP 用,如 "mcode acp")
    2: string command (api.json="command")
    // args 追加参数
    3: list<string> args (api.json="args")
    // cwd 工作目录(沙箱边界)
    4: string cwd (api.json="cwd")
    // timeoutSeconds 单轮超时
    5: i32 timeoutSeconds (api.json="timeout_seconds")
    // permission 授权档:ask|approve_reads|approve_all|deny
    6: string permission (api.json="permission")
    // mcpEnabled 是否把 git-ferry 自己的 /mcp 挂给外部 agent
    7: bool mcpEnabled (api.json="mcp_enabled")
    // mcpUrl git-ferry 自己的 MCP 端点地址
    8: string mcpUrl (api.json="mcp_url")
    // mcpApiKeyEnv MCP 专用 key 的环境变量名(该 key 固定为只读角色;密钥不进配置)
    9: string mcpApiKeyEnv (api.json="mcp_api_key_env")
    // mcpServers 额外挂给外部 agent 的 MCP 服务器(可多个)
    11: list<AIExternalMCPServer> mcpServers (api.json="mcp_servers")
    // agents 具名的外部 agent profile 列表(可同时挂 Claude Code + Codex …)
    12: list<AIExternalAgent> agents (api.json="agents")
    // current 驱动对话的 profile 名(留空=列表第一个)
    13: string current (api.json="current")
}

// AIExternalAgent 一个具名的外部 agent profile。
struct AIExternalAgent {
    1: string name (api.json="name")
    // transport acpx | stdio;留空=跟随 ai.backend
    2: string transport (api.json="transport")
    // agent acpx 内置 agent 名(mcode/codex/claude…)
    3: string agent (api.json="agent")
    // command 自定义命令(stdio 形态如 "mcode acp")
    4: string command (api.json="command")
    5: list<string> args (api.json="args")
    6: string cwd (api.json="cwd")
    7: i32 timeoutSeconds (api.json="timeout_seconds")
    // permission ask|approve_reads|approve_all|deny
    8: string permission (api.json="permission")
    9: bool mcpEnabled (api.json="mcp_enabled")
    10: string mcpUrl (api.json="mcp_url")
    11: string mcpApiKeyEnv (api.json="mcp_api_key_env")
    12: list<AIExternalMCPServer> mcpServers (api.json="mcp_servers")
}

// AIExternalMCPServer 额外挂给外部 agent 的一个 MCP 服务器。
struct AIExternalMCPServer {
    1: string name (api.json="name")
    2: string type (api.json="type")
    3: string url (api.json="url")
    // apiKeyEnv 认证头的**环境变量名**(密钥不进配置;变量里放完整头值)
    4: string apiKeyEnv (api.json="api_key_env")
    // apiKeyHeader 认证头名(留空=Authorization)
    5: string apiKeyHeader (api.json="api_key_header")
    6: string command (api.json="command")
    7: list<string> args (api.json="args")
    8: list<AIEnvVar> env (api.json="env")
}

// AIEnvVar 环境变量。
struct AIEnvVar {
    1: string name (api.json="name")
    2: string value (api.json="value")
}

struct AITestRequest {
    1: string baseUrl (api.json="base_url")
    2: optional string model (api.json="model")
    3: optional string apiKey (api.json="api_key")
    // backend 按哪个后端探测(留空=internal)
    4: optional string backend (api.json="backend")
    // external 外部 agent 配置(backend=acpx|stdio 时探测)
    5: optional AIExternalConfig external (api.json="external")
}

struct AIConfirmCall {
    1: string tool (api.json="tool")
    2: string token (api.json="token")
}

// AIPermissionCall 外部 agent 的授权回传(ACP session/request_permission)。
//
// 与 AIConfirmCall 是**两套不同的确认语义**:前者是 git-ferry 自己的
// 待确认令牌(危险工具),这里是 agent 侧的 request id → option id。
struct AIPermissionCall {
    1: string requestId (api.json="request_id")
    2: string optionId (api.json="option_id")
    3: optional string sessionId (api.json="session_id")
}

struct AIChatRequest {
    1: optional string sessionId (api.json="session_id")
    2: string message (api.json="message")
    3: optional AIConfirmCall confirmedToolCall (api.json="confirmed_tool_call")
    4: optional AIPermissionCall permissionCall (api.json="permission_call")
}
