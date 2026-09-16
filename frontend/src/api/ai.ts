import axios from 'axios'
import type { AIEvent, AIStatus } from '@/types/ai'
import { useAuthStore } from '@/stores/auth'

const statusHttp = axios.create({ baseURL: '/api/v1', timeout: 5000 })

/** 探测 AI 助手是否可用(未启用时后端 501)。 */
export async function getAIStatus(): Promise<AIStatus> {
  const auth = useAuthStore()
  const { data } = await statusHttp.get('/ai/status', {
    headers: { 'X-API-Key': auth.getApiKey() ?? '' },
  })
  return data?.data ?? { enabled: false }
}

export interface ChatStreamOptions {
  sessionId: string
  message: string
  confirmed?: { tool: string; token: string }
  signal?: AbortSignal
  onEvent: (ev: AIEvent) => void
}

/** POST /ai/chat 并解析 SSE 流。非 2xx 时抛错(带后端 message)。 */
export async function chatStream(opts: ChatStreamOptions): Promise<void> {
  const auth = useAuthStore()
  const resp = await fetch('/api/v1/ai/chat', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-API-Key': auth.getApiKey() ?? '',
    },
    body: JSON.stringify({
      session_id: opts.sessionId || undefined,
      message: opts.message,
      confirmed_tool_call: opts.confirmed,
    }),
    signal: opts.signal,
  })
  if (!resp.ok || !resp.body) {
    let msg = `AI 请求失败(HTTP ${resp.status})`
    try {
      const body = await resp.json()
      if (body?.message) msg = body.message
    } catch {
      /* 忽略解析失败 */
    }
    throw new Error(msg)
  }

  const reader = resp.body.getReader()
  const decoder = new TextDecoder()
  let buf = ''
  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    buf += decoder.decode(value, { stream: true })
    // SSE 帧以空行分隔
    const frames = buf.split('\n\n')
    buf = frames.pop() ?? ''
    for (const frame of frames) {
      let event = 'message'
      const dataLines: string[] = []
      for (const line of frame.split('\n')) {
        if (line.startsWith('event:')) event = line.slice(6).trim()
        else if (line.startsWith('data:')) dataLines.push(line.slice(5).trim())
      }
      if (!dataLines.length) continue
      try {
        const ev = JSON.parse(dataLines.join('\n')) as AIEvent
        ev.type = event as AIEvent['type']
        opts.onEvent(ev)
      } catch {
        /* 忽略坏帧 */
      }
    }
  }
}
