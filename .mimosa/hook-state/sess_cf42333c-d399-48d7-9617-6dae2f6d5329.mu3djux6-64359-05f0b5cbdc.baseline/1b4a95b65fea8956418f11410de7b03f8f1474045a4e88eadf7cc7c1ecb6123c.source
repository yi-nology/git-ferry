export interface Repo {
  id: number
  key: string
  name: string
  platform: string
  platform_owner: string
  platform_repo: string
  clone_url: string
  ssh_url: string
  default_branch: string
  status: string
  created_at: string
}

export interface SyncTask {
  id: number
  key: string
  name: string
  source_repo_key: string
  source_branch: string
  target_repo_key: string
  target_branch: string
  sync_mode: string
  cron: string
  webhook_token: string
  enabled: boolean
  git_tags: boolean
  git_force: boolean
  git_prune: boolean
  last_run_at: string
  last_status: string
  created_at: string
}

export interface SyncRun {
  id: number
  task_key: string
  trigger_source: string
  status: string
  start_time: string
  end_time: string
  commit_range: string
  details: string
  error_message: string
  created_at: string
}

export interface WebhookRule {
  id: number
  name: string
  repo_key: string
  event_type: string
  branch_pattern: string
  action: string
  sync_task_keys: string
  min_interval: number
  enabled: boolean
  description: string
  created_at: string
}

export interface WebhookEvent {
  id: number
  event_id: string
  repo_key: string
  event_type: string
  source: string
  actor_name: string
  branch: string
  commit_sha: string
  status: string
  error_message: string
  processed_at: string
  created_at: string
}

export interface CreateRepoRequest {
  name: string
  remote_url: string
  access_token?: string
}

export interface UpdateRepoRequest {
  key: string
  name?: string
  access_token?: string
}

export interface CreateTaskRequest {
  name: string
  source_repo_key: string
  source_branch?: string
  target_repo_key: string
  target_branch?: string
  sync_mode?: string
  cron?: string
  git_tags?: boolean
  git_force?: boolean
  git_prune?: boolean
}

export interface UpdateTaskRequest {
  key: string
  name?: string
  source_branch?: string
  target_branch?: string
  sync_mode?: string
  cron?: string
  enabled?: boolean
  git_tags?: boolean
  git_force?: boolean
  git_prune?: boolean
}

export interface CreateRuleRequest {
  name: string
  repo_key: string
  event_type?: string
  branch_pattern?: string
  action?: string
  sync_task_keys?: string
  min_interval?: number
  enabled?: boolean
  description?: string
}

export interface UpdateRuleRequest {
  id: number
  name?: string
  event_type?: string
  branch_pattern?: string
  action?: string
  sync_task_keys?: string
  min_interval?: number
  enabled?: boolean
  description?: string
}

// 平台侧 Webhook 注册(v1.6.0+)
export interface PlatformWebhookInfo {
  id: number
  url: string
  events: string[]
}

export interface RegisterPlatformWebhookRequest {
  repo_key: string
  callback_url: string
  secret?: string
  events?: string[]
}
