import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'
import 'dayjs/locale/zh-cn'

dayjs.extend(relativeTime)
dayjs.locale('zh-cn')

/** 相对时间（如"3分钟前"） */
export function formatTime(time: string | null | undefined): string {
  if (!time) return '-'
  try {
    return dayjs(time).fromNow()
  } catch {
    return time
  }
}

/** 绝对时间（如"2026-08-27 10:30:00"） */
export function formatDate(time: string | null | undefined): string {
  if (!time) return '-'
  try {
    return dayjs(time).format('YYYY-MM-DD HH:mm:ss')
  } catch {
    return time
  }
}

export function statusText(status: string | undefined): string {
  const map: Record<string, string> = {
    success: '成功',
    running: '运行中',
    failed: '失败',
    received: '已接收',
    processed: '已处理',
    active: '活跃',
    idle: '未运行',
    stopped: '已停止',
  }
  return map[status || ''] || status || '-'
}

export async function copyToClipboard(text: string): Promise<void> {
  await navigator.clipboard.writeText(text)
}

export function truncate(str: string, maxLen: number): string {
  if (!str) return ''
  return str.length > maxLen ? str.slice(0, maxLen) + '...' : str
}
