import api from './index'

export interface AIConfig {
  enabled: boolean
  base_url: string
  model: string
  temperature: number
  max_tokens: number
  timeout_seconds: number
  max_concurrent_chats: number
  has_api_key: boolean
  api_key_masked: string
  updated_at?: string
}

export interface AIConfigInput {
  enabled: boolean
  base_url: string
  model: string
  temperature?: number
  max_tokens?: number
  timeout_seconds?: number
  max_concurrent_chats?: number
  /** 空 = 沿用已保存密钥 */
  api_key?: string
}

export interface AITestResult {
  ok: boolean
  status: number
  model_ok: boolean
  message: string
  base_url: string
  checked_at: string
}

export const aiConfigApi = {
  get: () => api.get<unknown, AIConfig>('/ai/config'),
  update: (data: AIConfigInput) => api.post<unknown, AIConfig>('/ai/config', data),
  test: (data: { base_url: string; model: string; api_key?: string }) =>
    api.post<unknown, AITestResult>('/ai/config/test', data),
}
