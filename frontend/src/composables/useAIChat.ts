import { computed, ref } from 'vue'
import { chatStream, getAIStatus, respondPermission } from '@/api/ai'
import type {
  AIEvent,
  AIStatus,
  ChatMessage,
  PendingConfirm,
  PendingPermission,
} from '@/types/ai'

// 模块级单例:全局共享同一会话状态(App 只挂一个 AssistantPanel)。
const open = ref(false)
const enabled = ref<boolean | null>(null) // null = 未探测
const model = ref('')
const backend = ref('')
const backendLabel = ref('')
const ready = ref(true)
const readyMsg = ref('')
const sessionId = ref('')
const messages = ref<ChatMessage[]>([])
const streaming = ref(false)
const error = ref('')
const pendingConfirm = ref<PendingConfirm | null>(null)
const pendingPermission = ref<PendingPermission | null>(null)

let abort: AbortController | null = null

function handleEvent(ev: AIEvent) {
  switch (ev.type) {
    case 'start':
      sessionId.value = ev.session_id ?? sessionId.value
      break
    case 'delta': {
      const last = messages.value[messages.value.length - 1]
      if (last && last.role === 'assistant' && last.streaming) {
        last.content += ev.content ?? ''
      } else {
        messages.value.push({ role: 'assistant', content: ev.content ?? '', streaming: true })
      }
      break
    }
    case 'tool_start':
      messages.value.push({ role: 'tool', content: '调用中…', tool: ev.tool })
      break
    case 'tool_end': {
      // 提取结果摘要(错误/信息)附到工具行,直达执行的结果经此透出
      let extra = ''
      try {
        const r = ev.result ? (JSON.parse(ev.result) as Record<string, unknown>) : null
        if (r?.error) extra = `失败:${String(r.error).slice(0, 120)}`
        else if (r?.message) extra = String(r.message).slice(0, 80)
        else extra = '完成'
      } catch {
        extra = '完成'
      }
      for (let i = messages.value.length - 1; i >= 0; i--) {
        const m = messages.value[i]
        if (m.role === 'tool' && m.content === '调用中…') {
          m.content = extra
          break
        }
      }
      break
    }
    case 'tool_confirm':
      pendingConfirm.value = { tool: ev.tool ?? '', token: ev.token ?? '', args: ev.args }
      break
    case 'permission':
      // 外部 agent 请求授权(ACP session/request_permission)。
      pendingPermission.value = {
        requestId: ev.permission_id ?? '',
        tool: ev.tool ?? '',
        options: ev.options ?? [],
      }
      break
    case 'error':
      error.value = ev.content ?? '未知错误'
      messages.value.push({ role: 'error', content: error.value })
      break
    case 'done':
      streaming.value = false
      break
  }
}

export function useAIChat() {
  const toolActivity = computed(() => messages.value.filter((m) => m.role === 'tool'))

  async function loadStatus() {
    try {
      const st: AIStatus = await getAIStatus()
      enabled.value = st.enabled
      model.value = st.model ?? ''
      backend.value = st.backend ?? ''
      backendLabel.value = st.backend_label ?? ''
      ready.value = st.ready !== false
      readyMsg.value = st.ready_msg ?? ''
    } catch {
      enabled.value = false
    }
  }

  async function send(text: string, confirmed?: { tool: string; token: string }) {
    if (streaming.value) return
    if (!confirmed) messages.value.push({ role: 'user', content: text })
    error.value = ''
    pendingConfirm.value = null
    streaming.value = true
    abort = new AbortController()
    try {
      await chatStream({
        sessionId: sessionId.value,
        // 确认路径后端不经模型,message 仅要求非空
        message: confirmed ? ' ' : text,
        confirmed,
        signal: abort.signal,
        onEvent: handleEvent,
      })
    } catch (e) {
      if ((e as Error).name !== 'AbortError') {
        error.value = (e as Error).message
        messages.value.push({ role: 'assistant', content: `请求失败:${(e as Error).message}` })
      }
    } finally {
      messages.value.forEach((m) => (m.streaming = false))
      streaming.value = false
      abort = null
    }
  }

  function confirm() {
    if (!pendingConfirm.value || streaming.value) return
    const { tool, token } = pendingConfirm.value
    pendingConfirm.value = null
    void send('', { tool, token })
  }

  function deny() {
    pendingConfirm.value = null
    messages.value.push({ role: 'assistant', content: '已取消,该操作未执行。' })
  }

  /**
   * 回传外部 agent 的授权决定。
   *
   * optionID 为空 = 拒绝。回传**不影响**正在进行的 SSE 流 ——
   * 输出仍在原来那条流里继续。
   */
  async function answerPermission(optionId: string) {
    if (!pendingPermission.value) return
    const { requestId } = pendingPermission.value
    pendingPermission.value = null
    try {
      await respondPermission({
        request_id: requestId,
        option_id: optionId,
        session_id: sessionId.value || undefined,
      })
    } catch (e) {
      error.value = (e as Error).message
      messages.value.push({ role: 'error', content: `授权回传失败:${(e as Error).message}` })
    }
  }

  function cancelStream() {
    abort?.abort()
  }

  function reset() {
    cancelStream()
    sessionId.value = ''
    messages.value = []
    pendingConfirm.value = null
    pendingPermission.value = null
    error.value = ''
  }

  function toggle() {
    open.value = !open.value
    if (open.value && enabled.value === null) void loadStatus()
  }

  return {
    open,
    enabled,
    model,
    backend,
    backendLabel,
    ready,
    readyMsg,
    sessionId,
    messages,
    streaming,
    error,
    pendingConfirm,
    pendingPermission,
    toolActivity,
    loadStatus,
    send,
    confirm,
    deny,
    answerPermission,
    cancelStream,
    reset,
    toggle,
  }
}
