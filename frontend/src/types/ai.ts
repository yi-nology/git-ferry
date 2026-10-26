export interface AIStatus {
  enabled: boolean
  model?: string
  /** 执行后端:internal | acpx | stdio */
  backend?: string
  /** 后端展示名(如 "外部 coding agent (mcode / ACP)") */
  backend_label?: string
  /** 外部后端的静态就位性(装没装) */
  ready?: boolean
  /** 就位性说明(就位原因或缺什么) */
  ready_msg?: string
}

export type AIEventType =
  | 'start'
  | 'delta'
  | 'thinking'
  | 'plan'
  | 'tool_start'
  | 'tool_end'
  | 'tool_confirm'
  | 'permission'
  | 'agent_session'
  | 'done'
  | 'error'

/** 外部 agent 给出的一个授权选项(ACP session/request_permission)。 */
export interface AIPermissionOption {
  option_id: string
  name: string
  /** allow_once | allow_always | reject_once | reject_always */
  kind?: string
}

export interface AIEvent {
  type: AIEventType
  content?: string
  tool?: string
  args?: string
  result?: string
  token?: string
  session_id?: string
  usage?: { input_tokens: number; output_tokens: number }
  /** 外部 agent 侧的会话 ID(可在 agent 自己的界面里找到这次会话) */
  agent_session_id?: string
  /** 外部 agent 的展示名 */
  agent_label?: string
  /** 授权请求 id(回传时原样带回) */
  permission_id?: string
  /** 授权选项 */
  options?: AIPermissionOption[]
}

export interface ChatMessage {
  role: 'user' | 'assistant' | 'tool' | 'error'
  content: string
  tool?: string
  streaming?: boolean
  /** 外部 agent 委派任务 ID(有它说明这行在等外部 agent 的结果) */
  taskId?: string
  /** 外部 agent 的思考过程(折叠展示,按上限截断) */
  thinking?: string
  /** 思考过程是否被截断 */
  thinkingTruncated?: boolean
}

export interface PendingConfirm {
  tool: string
  token: string
  args?: string
}

/** 外部 agent 的待授权请求。 */
export interface PendingPermission {
  requestId: string
  tool: string
  options: AIPermissionOption[]
}
