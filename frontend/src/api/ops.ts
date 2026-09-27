import http from './http'

/** 健康评分单项 */
export interface HealthScoreItem {
  key: string
  name: string
  score: number
  level: 'gold' | 'silver' | 'bronze' | 'basic'
  issues?: string[]
}

export interface HealthScoreData {
  items: HealthScoreItem[]
  total: number
}

/** 仓库资产盘点项 */
export interface InventoryItem {
  repo_key: string
  repo_name: string
  platform?: string
  status?: string
  has_task: boolean
  task_keys?: string[]
  last_run_status?: string
  last_run_at?: string
  coverage: 'covered' | 'no_task' | 'stale' | 'failing'
}

export interface InventoryData {
  items: InventoryItem[]
  total: number
  covered: number
  orphan_repos: number
  failing: number
}

/** 同步策略模板 */
export interface SyncTemplate {
  id: string
  name: string
  description?: string
  match?: Record<string, string[]>
  spec: {
    cron?: string
    source_branch?: string
    target_branch?: string
    enabled?: boolean
    timeout_seconds?: number
    retry_max?: number
  }
  tags?: string[]
  created_at?: string
  updated_at?: string
}

export interface TemplateListData {
  items: SyncTemplate[]
}

export interface TemplateApplyResult {
  template: SyncTemplate
  changed: Array<{ key: string; name: string; before: Record<string, unknown>; after: Record<string, unknown> }>
  total: number
  dry_run: boolean
}

export interface OverviewData {
  repo_count: number
  tasks_by_status: Record<string, number>
  recent_failed: Array<{
    run_id: number
    task_key: string
    error_msg: string
    err_type: string
    end_at: string
  }>
  health: Record<string, string>
  generated_at: string
}

export interface DeployKeyResult {
  private_key_pem: string
  public_key: string
  fingerprint: string
  comment: string
  note?: string
}

export interface BundleInfo {
  name: string
  path: string
  size: number
  mod_time: string
  valid?: boolean
  ref_specs?: string[]
}

export const opsApi = {
  overview: () => http.get<unknown, OverviewData>('/ops/overview'),
  healthScore: (limit?: number) =>
    http.get<unknown, HealthScoreData>('/ops/health-score', { params: { limit } }),
  inventory: () => http.get<unknown, InventoryData>('/ops/inventory'),
  listTemplates: () => http.get<unknown, TemplateListData>('/ops/templates'),
  createTemplate: (data: Partial<SyncTemplate>) =>
    http.post<unknown, SyncTemplate>('/ops/templates', data),
  deleteTemplate: (id: string) =>
    http.post<unknown, { success: boolean }>('/ops/templates/delete', null, { params: { id } }),
  previewTemplate: (template_id: string) =>
    http.post<unknown, { matched: Array<{ key: string; name: string }>; total: number }>(
      '/ops/templates/preview', { template_id },
    ),
  applyTemplate: (template_id: string, dry_run = true) =>
    http.post<unknown, TemplateApplyResult>('/ops/templates/apply', { template_id, dry_run }),
  retryRun: (run_id: number) =>
    http.post<unknown, { success: boolean; task_key: string }>('/ops/retry', { run_id }),
  retryBatch: (limit?: number, task_key?: string) =>
    http.post<unknown, { retried: Array<{ run_id: number; task_key: string }>; skipped: number; candidate: number }>(
      '/ops/retry-batch', { limit, task_key },
    ),
  generateDeployKey: (comment: string) =>
    http.post<unknown, DeployKeyResult>('/ops/deploy-key', { comment }),
  listBundles: (task_key?: string) =>
    http.get<unknown, { items: BundleInfo[]; total: number; backup_dir: string }>('/ops/bundles', { params: { task_key } }),
  verifyBundle: (name: string) =>
    http.get<unknown, BundleInfo>('/ops/bundles/verify', { params: { name } }),
  restoreBundle: (name: string, dest_dir: string) =>
    http.post<unknown, { success: boolean; dest: string }>('/ops/bundles/restore', { name, dest_dir }),
  diagnoseRun: (run_id: number) =>
    http.get<unknown, Record<string, unknown>>('/ops/diagnose', { params: { run_id } }),
  auditReport: (params?: { format?: 'json' | 'csv'; limit?: number; action?: string }) =>
    http.get<unknown, { items: unknown[]; total: number }>('/ops/audit-report', { params }),
}
