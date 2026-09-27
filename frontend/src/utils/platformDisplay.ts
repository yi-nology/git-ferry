import { markRaw } from 'vue'
import {
  GithubOutlined,
  GitlabOutlined,
  CloudOutlined,
  CodeOutlined,
  AppstoreOutlined,
  QqOutlined,
  SettingOutlined,
} from '@ant-design/icons-vue'
import { PLATFORM_COLOR } from '@/utils/platform'
import type { Platform } from '@/api/platform'

const ICONS: Record<string, unknown> = {
  github: GithubOutlined,
  gitlab: GitlabOutlined,
  gitea: CloudOutlined,
  gitee: CloudOutlined,
  gitcode: CodeOutlined,
  atomgit: AppstoreOutlined,
  tencent_code: QqOutlined,
  custom: SettingOutlined,
}

export function platformColor(type: string): string {
  return PLATFORM_COLOR[type] || '#666666'
}

export function platformIcon(type: string) {
  return markRaw((ICONS[type] || SettingOutlined) as never)
}

/** 兼容 camelCase/snake_case 字段 */
export function getInstanceUrl(p: Platform): string {
  return p.instanceUrl || p.instance_url || ''
}
export function getApiUrl(p: Platform): string {
  return p.apiUrl || p.api_url || ''
}
export function getSkipTls(p: Platform): boolean {
  return p.skip_tls_verify || p.skipTlsVerify || false
}
export function getIsDefault(p: Platform): boolean {
  return p.is_default || p.isDefault || false
}
export function getRepoCount(p: Platform): number {
  return p.repo_count || p.repoCount || 0
}
export function getLastTestAt(p: Platform): string {
  return p.last_test_at || p.lastTestAt || ''
}
