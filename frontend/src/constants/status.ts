/** 状态字典 —— StatusBadge 组件与 statusText 工具函数的统一来源 */
/* eslint-disable @typescript-eslint/consistent-indexed-object-style */
// 用 Record<string, ...> 而非 Record<StatusValue, ...>:入参来自后端任意 string,
// 索引函数已做兜底(未知状态原样展示/归 idle),无需收窄类型

export type StatusKind = 'success' | 'running' | 'failed' | 'warning' | 'idle'
export type StatusValue = 'success' | 'running' | 'failed' | 'received' | 'processed' | 'active' | 'idle' | 'stopped' | 'pending' | 'error' | 'warning' | 'disabled'

export const STATUS_LABEL: Record<string, string> = {
  success: '成功',
  running: '运行中',
  failed: '失败',
  received: '已接收',
  processed: '已处理',
  active: '活跃',
  idle: '未运行',
  stopped: '已停止',
  pending: '等待中',
  error: '错误',
  warning: '警告',
  disabled: '已禁用',
}

/** 把业务状态映射到展示样式分类 */
export const STATUS_KIND: Record<string, StatusKind> = {
  success: 'success',
  running: 'running',
  failed: 'failed',
  received: 'running',
  processed: 'success',
  active: 'success',
  idle: 'idle',
  stopped: 'idle',
  disabled: 'idle',
  error: 'failed',
  warning: 'warning',
  pending: 'running',
}

export function statusLabel(status?: string): string {
  if (!status) return STATUS_LABEL.idle
  return STATUS_LABEL[status] || status
}

export function statusKind(status?: string): StatusKind {
  if (!status) return 'idle'
  return STATUS_KIND[status] || 'idle'
}


/** 展示用颜色(Ant Design tag color),与 STATUS_KIND 对齐 */
export const STATUS_COLOR: Record<string, string> = {
  success: 'green',
  running: 'blue',
  failed: 'red',
  received: 'blue',
  processed: 'green',
  active: 'green',
  idle: 'default',
  stopped: 'default',
  disabled: 'default',
  error: 'red',
  warning: 'orange',
  pending: 'blue',
}

export function statusColor(status?: string): string {
  if (!status) return 'default'
  return STATUS_COLOR[status] || 'default'
}

/** 常用状态字面量,避免魔法字符串散落 */
export const STATUS = {
  Success: 'success',
  Running: 'running',
  Failed: 'failed',
  Idle: 'idle',
} as const
