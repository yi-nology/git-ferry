import http from './http'

/** 健康维度（Scorecards 风格） */
export interface HealthDimension {
  name: 'reliability' | 'freshness' | 'schedule' | 'safety' | 'completeness' | string
  weight: number
  score: number
  reason?: string
  detail?: string[]
  action?: string
}

/** 健康评分单项 */
export interface HealthScoreItem {
  key: string
  name: string
  score: number
  level: 'gold' | 'silver' | 'bronze' | 'basic'
  issues?: string[]
  actions?: string[]
  dimensions?: HealthDimension[]
}

export interface HealthScoreData {
  items: HealthScoreItem[]
  total: number
  attention?: HealthScoreItem[]
  summary?: {
    levels?: Record<string, number>
    below_silver?: number
    top_actions?: string[]
  }
  generated_at?: string
}

/** 统一待办动作（可复制命令） */
export interface OpsTodoAction {
  kind: 'cli' | 'manual' | string
  dimension?: string
  priority?: number
  title: string
  command?: string
  danger?: boolean
  reason?: string
}

export interface OpsTodoItem {
  id: string
  kind: 'health' | 'orphan' | 'rpo' | 'drift' | string
  priority: number // 1=紧急
  task_key?: string
  repo_key?: string
  title: string
  reason?: string
  actions?: OpsTodoAction[]
}

export interface OpsTodoData {
  items: OpsTodoItem[]
  total: number
  by_kind?: Record<string, number>
  generated_at?: string
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
  /** 继承的基础模板 ID（Renovate preset 模式） */
  extends?: string
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
  effective_spec?: SyncTemplate['spec']
  extends_chain?: string[]
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
  opsTodo: () => http.get<unknown, OpsTodoData>('/ops/todo'),
  healthScore: (limit?: number, opts?: { withDrift?: boolean }) =>
    http.get<unknown, HealthScoreData>('/ops/health-score', {
      params: { limit, with_drift: opts?.withDrift ? 1 : undefined },
    }),
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

  // P0 灾备闭环
  runDRDrill: (name?: string, all = false, max = 5) =>
    http.post<unknown, { mode: string; reports: DrillReport[]; summary?: Record<string, unknown> }>(
      '/ops/dr-drill', { name, all, max },
    ),
  drillHistory: (limit = 20) =>
    http.get<unknown, { items: DrillHistoryEntry[]; total: number }>('/ops/dr-drill/history', { params: { limit } }),
  exportDrillHistory: async (format: 'json' | 'csv' = 'json', limit = 100): Promise<Blob> => {
    const resp = await http.get(`/ops/dr-drill/export`, {
      params: { format, limit },
      responseType: 'blob',
    })
    return resp as unknown as Blob
  },
  verifyDrillChain: () =>
    http.get<unknown, { ok: boolean; checked: number; broken: string }>('/ops/dr-drill/chain/verify'),
  buildBackupManifest: () =>
    http.post<unknown, BackupManifest>('/ops/backup-manifest', {}),
  verifyBackupManifest: () =>
    http.get<unknown, ManifestVerifyResult>('/ops/backup-manifest/verify'),
  rpoReport: (maxSeconds = 0) =>
    http.get<unknown, RPOReport>('/ops/rpo', { params: { max_seconds: maxSeconds } }),

  // P1 元数据资产
  metadataBackup: (repo_key: string, opts?: { with_archives?: boolean; with_assets?: boolean; with_gists?: boolean }) =>
    http.post<unknown, MetadataSnapshot>('/ops/metadata-backup', { repo_key, ...opts }),
  listMetadataBackups: (repo_key?: string) =>
    http.get<unknown, { items: MetadataSnapshot[]; total: number }>('/ops/metadata-backups', { params: { repo_key } }),
  backupGists: (platform_key: string, max_gists = 200) =>
    http.post<unknown, { platform_key: string; count: number; dir: string; warnings?: string[] }>(
      '/ops/gists-backup', { platform_key, max_gists },
    ),

  // P3 生命周期
  autoDiscover: (platform_key: string, import_new = false) =>
    http.post<unknown, DiscoveryReport>('/ops/auto-discover', { platform_key, import_new }),
  detectDrift: (task_keys?: string[]) =>
    http.post<unknown, DriftReport>('/ops/drift', { task_keys }),
  cleanupBackups: () =>
    http.post<unknown, { removed: number; legal_hold: boolean }>('/ops/backup-cleanup', { confirm: 'yes' }),

  // P4 治理
  verifyAuditChain: () =>
    http.get<unknown, { ok: boolean; checked: number; broken_at?: number; message: string }>('/ops/audit-chain/verify'),
  rbac: () =>
    http.get<unknown, { role: string; user: string; permissions: Record<string, boolean> }>('/ops/rbac'),
}

/** DR 演练报告 */
export interface DrillReport {
  bundle_name: string
  started_at: string
  finished_at: string
  duration_ms: number
  success: boolean
  ref_specs?: string[]
  restored_refs?: string[]
  missing_refs?: string[]
  extra_refs?: string[]
  fsck_ok: boolean
  fsck_output?: string
  commit_count: number
  est_rto: string
  bundle_size: number
  errors?: string[]
}

export interface DrillHistoryEntry {
  report: DrillReport
  prev_hash: string
  hash: string
}

export interface BackupManifest {
  version: number
  generated: string
  backup_dir: string
  entry_count: number
  total_size: number
  merkle_root: string
  entries: Array<{ name: string; size: number; sha256: string; mod_time: string; leaf_hash: string }>
}

export interface ManifestVerifyResult {
  ok: boolean
  merkle_root: string
  stored_root?: string
  checked_count: number
  missing_files?: string[]
  hash_mismatch?: string[]
  extra_files?: string[]
  message: string
}

export interface RPOMetric {
  task_key: string
  latest_bundle: string
  last_backup_at: string
  bundle_count: number
  total_size: number
  rpo_seconds: number
  rpo_human: string
  rpo_violated: boolean
  est_rto: string
}

export interface RPOReport {
  generated: string
  backup_dir: string
  rpo_max_seconds: number
  overall_rpo_seconds: number
  overall_rpo_human: string
  worst_task: string
  violations: number
  metrics: RPOMetric[]
}

export interface MetadataSnapshot {
  repo_key: string
  platform: string
  owner: string
  repo: string
  created_at: string
  dir: string
  counts: Record<string, number>
  files: string[]
  archives?: string[]
  warnings?: string[]
}

export interface DiscoveryReport {
  platform_key: string
  scanned_at: string
  found: number
  existing: number
  new_repos: string[]
  imported: number
  warnings?: string[]
}

export interface DriftItem {
  task_key: string
  repo_key: string
  branch: string
  local_ref: string
  remote_ref?: string
  drifted: boolean
  local_ahead: number
  remote_ahead: number
  message: string
}

export interface DriftReport {
  generated: string
  checked: number
  drifted: number
  items: DriftItem[]
  warnings?: string[]
}
