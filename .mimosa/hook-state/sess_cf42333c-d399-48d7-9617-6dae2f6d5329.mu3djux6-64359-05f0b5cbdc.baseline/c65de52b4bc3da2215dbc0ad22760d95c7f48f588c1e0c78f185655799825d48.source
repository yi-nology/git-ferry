/** 平台展示字典 —— 全局唯一来源,消除各页面重复硬编码 */

export const PLATFORM_LABEL: Record<string, string> = {
  github: 'GitHub',
  gitlab: 'GitLab',
  gitea: 'Gitea',
  gitee: 'Gitee',
  gitcode: 'GitCode',
  atomgit: 'AtomGit',
  tencent_code: '腾讯工蜂',
}

export const PLATFORM_COLOR: Record<string, string> = {
  github: '#24292E',
  gitlab: '#FC6D26',
  gitea: '#609926',
  gitee: '#C71D23',
  gitcode: '#0066FF',
  atomgit: '#0084FF',
  tencent_code: '#00B4D8',
}

export function platformLabel(platform?: string): string {
  if (!platform) return '-'
  return PLATFORM_LABEL[platform] || platform
}

// 始终返回 hex,既可用于 a-tag :color,也可用于 :style background
export function platformColor(platform?: string): string {
  if (!platform) return '#1677FF'
  return PLATFORM_COLOR[platform] || '#1677FF'
}
