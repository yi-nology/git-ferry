export interface AIStatus {
  enabled: boolean
  model?: string
}

export type AIEventType =
  | 'start'
  | 'delta'
  | 'tool_start'
  | 'tool_end'
  | 'tool_confirm'
  | 'done'
  | 'error'

export interface AIEvent {
  type: AIEventType
  content?: string
  tool?: string
  args?: string
  result?: string
  token?: string
  session_id?: string
  usage?: { input_tokens: number; output_tokens: number }
}

export interface ChatMessage {
  role: 'user' | 'assistant' | 'tool' | 'error'
  content: string
  tool?: string
  streaming?: boolean
}

export interface PendingConfirm {
  tool: string
  token: string
  args?: string
}
