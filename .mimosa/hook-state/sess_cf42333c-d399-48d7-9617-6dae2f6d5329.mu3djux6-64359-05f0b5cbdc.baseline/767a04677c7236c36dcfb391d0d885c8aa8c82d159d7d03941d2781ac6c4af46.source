/** 触发来源 / 事件类型 等业务字典 —— 消除多页面重复定义 */

export const TRIGGER_LABEL: Record<string, string> = {
  manual: '手动',
  schedule: '定时',
  cron: '定时',
  webhook: 'Webhook',
  api: 'API',
  retry: '重试',
}

export const TRIGGER_COLOR: Record<string, string> = {
  manual: 'blue',
  schedule: 'cyan',
  cron: 'cyan',
  webhook: 'purple',
  api: 'geekblue',
  retry: 'orange',
}

export function triggerLabel(t?: string): string {
  return t ? TRIGGER_LABEL[t] || t : '-'
}

export function triggerColor(t?: string): string {
  return t ? TRIGGER_COLOR[t] || 'default' : 'default'
}

export const EVENT_TYPE_COLOR: Record<string, string> = {
  push: 'green',
  pull_request: 'blue',
  merge_request: 'blue',
  tag: 'purple',
  tag_push: 'gold',
  create: 'cyan',
  delete: 'red',
}

export function eventTypeColor(t?: string): string {
  return t ? EVENT_TYPE_COLOR[t] || 'default' : 'default'
}
