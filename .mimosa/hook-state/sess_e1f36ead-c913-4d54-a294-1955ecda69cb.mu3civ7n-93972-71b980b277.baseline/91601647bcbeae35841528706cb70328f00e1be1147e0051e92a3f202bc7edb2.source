/**
 * API 层类型定义
 *
 * 后端约定:所有 `/api/v1/*` 响应统一包装为
 *   { code, message, data, timestamp }
 * 其中 delete 类操作返回 204 空体;认证中间件 401 返回 { error: "..." }(非标准包装)。
 * 拦截器(api/http.ts)已统一解包,业务代码拿到的就是下方各 `*Data` 类型。
 */
import type {
  Repo,
  SyncTask,
  SyncRun,
  WebhookRule,
  WebhookEvent,
} from './index'

/** 后端统一响应包装(拦截器解包前) */
export interface ApiResponse<T = unknown> {
  code: number
  message: string
  data: T
  timestamp: number
}

/** 请求侧分页参数(字段可选) */
export interface PageParams {
  page?: number
  page_size?: number
}

/** 响应侧分页对象(repos / logs 使用) */
export interface PaginationInfo {
  page: number
  page_size: number
  total: number
  total_pages?: number
}

/* ---------- 各接口的精确 data 类型 ---------- */

export interface RepoListData {
  list: Repo[]
  pagination: PaginationInfo
}
export interface RepoData {
  repo: Repo
}
export interface BranchesData {
  branches: string[]
}
export interface TestConnectionData {
  success: boolean
  message: string
}

export interface TaskListData {
  tasks: SyncTask[]
  total: number
}
export interface TaskData {
  task: SyncTask
}
export interface RunResultData {
  success: boolean
  message: string
}
export interface HistoryData {
  runs: SyncRun[]
}
export interface PreviewSyncData {
  can_sync: boolean
  source_exists: boolean
  target_exists: boolean
  message: string
}

export interface RulesData {
  rules: WebhookRule[]
}
export interface RuleData {
  rule: WebhookRule
}
export interface EventsData {
  events: WebhookEvent[]
}

/** 批量操作结果(repos batch) */
export interface BatchResultData {
  total: number
  success: number
  failed: number
  errors?: string[]
}

/** 204 空体 / 简单成功响应的兜底 */
export interface SuccessData {
  success: boolean
}

/* ---------- System ---------- */

export interface SystemStatusData {
  status: string
  version: string
  uptime: number
  repo_count: number
  task_count: number
  running_task: number
  failed_task: number
  go_version: string
  platform: string
}
export interface HealthData {
  status: string
  database: {
    status: string
    size: number
  }
}

/* ---------- Logs ---------- */

export interface OperationLog {
  id: number
  time: string
  user: string
  action: string
  resource: string
  details: string
  ip: string
}
export interface OperationLogStats {
  today: number
  week: number
  total: number
}
export interface OperationLogListData {
  list: OperationLog[]
  pagination: PaginationInfo
  stats: OperationLogStats
}
export interface OperationLogParams extends PageParams {
  search?: string
  action?: string
  user?: string
  start_date?: string
  end_date?: string
}

export interface SyncLog {
  id: number
  task_key: string
  trigger_source: string
  status: string
  start_time: string
  end_time: string
  commit_range: string
  details: string
  error_message: string
}
export interface SyncLogListData {
  list: SyncLog[]
  pagination: PaginationInfo
}
export interface SyncLogParams extends PageParams {
  task_key?: string
  status?: string
}

export interface SystemLog {
  id: number
  time: string
  level: string
  message: string
  details: string
}
export interface SystemLogListData {
  list: SystemLog[]
  pagination: PaginationInfo
}
export interface SystemLogParams extends PageParams {
  level?: string
}

export interface PlatformWebhookData {
  webhook: { id: number; url: string; events: string[] }
}

export interface PlatformWebhookListData {
  webhooks: { id: number; url: string; events: string[] }[]
}
