/** 状态字典 —— StatusBadge 组件与 statusText 工具函数的统一来源 */

export type StatusKind = 'success' | 'running' | 'failed' | 'warning' | 'idle'
export type StatusValue = 'success' | 'running' | 'failed' | 'received' | 'processed' | 'active' | 'idle' | 'stopped' | 'pending' | 'error' | 'warning' | 'disabled'

export const STATUS_LABEL: Record<StatusValue, string> = {
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
export const STATUS_KIND: Record<StatusValue, StatusKind> = {
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
