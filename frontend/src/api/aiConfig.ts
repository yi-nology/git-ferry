import api from './index'

/** 执行后端:内置助手 / 外部 coding agent。 */
export type AIBackend = 'internal' | 'acpx' | 'stdio'

/** 额外挂给外部 agent 的一个 MCP 服务器。 */
export interface AIExternalMCPServer {
  name: string
  /** http | sse | stdio(留空=stdio) */
  type: string
  url: string
  /** 认证头的环境变量名(变量里放完整头值);留空=不带认证 */
  api_key_env: string
  /** 认证头名;留空=Authorization */
  api_key_header: string
  command: string
  args: string[]
  env: { name: string; value: string }[]
}

/** 外部 coding agent 配置。 */
export interface AIExternalConfig {
  agent: string
  command: string
  args: string[]
  cwd: string
  timeout_seconds: number
  /** ask | approve_reads | approve_all | deny */
  permission: string
  mcp_enabled: boolean
  mcp_url: string
  /** MCP 专用 key 的环境变量名(该 key 固定只读角色;密钥不进配置) */
  mcp_api_key_env: string
  /** 额外挂给外部 agent 的 MCP 服务器 */
  mcp_servers: AIExternalMCPServer[]
}

export interface AIConfig {
  enabled: boolean
  backend: AIBackend
  base_url: string
  model: string
  temperature: number
  max_tokens: number
  timeout_seconds: number
  max_concurrent_chats: number
  has_api_key: boolean
  api_key_masked: string
  updated_at?: string
  external: AIExternalConfig
}

export interface AIConfigInput {
  enabled: boolean
  backend: AIBackend
  base_url: string
  model: string
  temperature?: number
  max_tokens?: number
  timeout_seconds?: number
  max_concurrent_chats?: number
  /** 空 = 沿用已保存密钥 */
  api_key?: string
  external: AIExternalConfig
}

export interface AITestResult {
  ok: boolean
  status: number
  model_ok: boolean
  message: string
  base_url: string
  backend?: string
  checked_at: string
}

export interface AIModelItem {
  id: string
  owned_by?: string
}

export interface AIModelList {
  models: AIModelItem[]
  total: number
  base_url: string
}

export const aiConfigApi = {
  get: () => api.get<unknown, AIConfig>('/ai/config'),
  update: (data: AIConfigInput) => api.post<unknown, AIConfig>('/ai/config', data),
  test: (data: {
    base_url: string
    model: string
    api_key?: string
    backend?: AIBackend
    external?: AIExternalConfig
  }) => api.post<unknown, AITestResult>('/ai/config/test', data),
  // POST + body:api_key 走请求体而不是 query(query 会进浏览器历史/Referer/代理日志)
  listModels: (params: { base_url: string; api_key?: string }) =>
    api.post<unknown, AIModelList>('/ai/models', {
      base_url: params.base_url,
      api_key: params.api_key || undefined,
    }),
}
