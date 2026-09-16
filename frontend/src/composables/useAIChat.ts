import { computed, ref } from 'vue'
import { chatStream, getAIStatus } from '@/api/ai'
import type { AIEvent, AIStatus, ChatMessage, PendingConfirm } from '@/types/ai'

// 模块级单例:全局共享同一会话状态(App 只挂一个 AssistantPanel)。
const open = ref(false)
const enabled = ref<boolean | null>(null) // null = 未探测
const model = ref('')
const sessionId = ref('')
const messages = ref<ChatMessage[]>([])
const streaming = ref(false)
const error = ref('')
const pendingConfirm = ref<PendingConfirm | null>(null)

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
      messages.value.push({ role: 'tool', content: `调用 ${ev.tool}…`, tool: ev.tool })
      break
    case 'tool_end': {
      // 提取结果摘要(错误/信息)附到工具行,直达执行的结果经此透出
      let extra = ''
      try {
        const r = ev.result ? (JSON.parse(ev.result) as Record<string, unknown>) : null
        if (r?.error) extra = ` · 失败:${String(r.error).slice(0, 120)}`
        else if (r?.message) extra = ` · ${String(r.message).slice(0, 80)}`
      } catch {
        /* result 非 JSON,忽略 */
      }
      for (let i = messages.value.length - 1; i >= 0; i--) {
        const m = messages.value[i]
        if (m.role === 'tool' && m.content.endsWith('…')) {
          m.content = `${m.tool ?? ev.tool ?? '工具'} 完成${extra}`
          break
        }
      }
      break
    }
    case 'tool_confirm':
      pendingConfirm.value = { tool: ev.tool ?? '', token: ev.token ?? '', args: ev.args }
      break
    case 'error':
      error.value = ev.content ?? '未知错误'
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

  function cancelStream() {
    abort?.abort()
  }

  function reset() {
    cancelStream()
    sessionId.value = ''
    messages.value = []
    pendingConfirm.value = null
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
    sessionId,
    messages,
    streaming,
    error,
    pendingConfirm,
    toolActivity,
    loadStatus,
    send,
    confirm,
    deny,
    cancelStream,
    reset,
    toggle,
  }
}
