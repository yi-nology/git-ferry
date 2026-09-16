import http from './http'
import type {
  PageParams,
  RepoListData,
  RepoData,
  BranchesData,
  TestConnectionData,
  BatchResultData,
  SuccessData,
  TaskListData,
  TaskData,
  RunResultData,
  HistoryData,
  PreviewSyncData,
  RulesData,
  RuleData,
  EventsData,
  SystemStatusData,
  HealthData,
  OperationLogListData,
  OperationLogParams,
  SyncLogListData,
  SyncLogParams,
  SystemLogListData,
  SystemLogParams,
  PlatformWebhookData,
  PlatformWebhookListData,
} from '@/types/api'
import type {
  CreateRepoRequest,
  UpdateRepoRequest,
  CreateTaskRequest,
  UpdateTaskRequest,
  CreateRuleRequest,
  UpdateRuleRequest,
} from '@/types'

/** 仓库列表查询参数:分页 + 过滤(后端 /repos 支持) */
export interface RepoQueryParams extends PageParams {
  search?: string
  platform?: string
  status?: string
}

export const repoApi = {
  list: (params?: RepoQueryParams) =>
    http.get<unknown, RepoListData>('/repos', { params }),
  get: (key: string) =>
    http.get<unknown, RepoData>('/repo', { params: { key } }),
  create: (data: CreateRepoRequest) =>
    http.post<unknown, RepoData>('/repo/create', data),
  update: (data: UpdateRepoRequest) =>
    http.post<unknown, RepoData>('/repo/update', data),
  delete: (key: string) =>
    http.post<unknown, SuccessData>('/repo/delete', null, { params: { key } }),
  batchDelete: (keys: string[]) =>
    http.post<unknown, BatchResultData>('/repos/batch', { action: 'delete', keys }),
  testConnection: (key: string) =>
    http.post<unknown, TestConnectionData>('/repo/test', null, { params: { key } }),
  listBranches: (key: string) =>
    http.get<unknown, BranchesData>('/repo/branches', { params: { key } }),
}

export interface TaskListParams extends PageParams {
  repo_key?: string
  status?: string
  search?: string
}

export const syncTaskApi = {
  list: (params?: TaskListParams) =>
    http.get<unknown, TaskListData>('/sync/tasks', { params }),
  get: (key: string) =>
    http.get<unknown, TaskData>('/sync/task', { params: { key } }),
  create: (data: CreateTaskRequest) =>
    http.post<unknown, TaskData>('/sync/task/create', data),
  update: (data: UpdateTaskRequest) =>
    http.post<unknown, TaskData>('/sync/task/update', data),
  delete: (key: string) =>
    http.post<unknown, SuccessData>('/sync/task/delete', null, { params: { key } }),
  run: (key: string) =>
    http.post<unknown, RunResultData>('/sync/task/run', null, { params: { key } }),
  preview: (data: {
    source_repo_key: string
    source_branch?: string
    target_repo_key: string
    target_branch?: string
  }) => http.post<unknown, PreviewSyncData>('/sync/preview', data),
  history: (params: { task_key: string; limit?: number }) =>
    http.get<unknown, HistoryData>('/sync/history', { params }),
  deleteHistory: (id: number) =>
    http.post<unknown, { message: string }>('/sync/history/delete', null, { params: { id } }),
}

export const webhookApi = {
  listRules: (repo_key: string) =>
    http.get<unknown, RulesData>('/webhook/rules', { params: { repo_key } }),
  getRule: (id: number) =>
    http.get<unknown, RuleData>('/webhook/rule', { params: { id } }),
  createRule: (data: CreateRuleRequest) =>
    http.post<unknown, RuleData>('/webhook/rule/create', data),
  updateRule: (data: UpdateRuleRequest) =>
    http.post<unknown, RuleData>('/webhook/rule/update', data),
  deleteRule: (id: number) =>
    http.post<unknown, SuccessData>('/webhook/rule/delete', null, { params: { id } }),
  listEvents: (params: { repo_key: string; limit?: number }) =>
    http.get<unknown, EventsData>('/webhook/events', { params }),
  retryEvent: (id: number) =>
    http.post<unknown, RunResultData>('/webhook/event/retry', null, { params: { id } }),
  // 平台侧 Webhook 注册(v1.6.0+)
  registerPlatformWebhook: (data: { repo_key: string; callback_url: string; secret?: string; events?: string[] }) =>
    http.post<unknown, PlatformWebhookData>('/webhook/platform/register', data),
  listPlatformWebhooks: (repo_key: string) =>
    http.get<unknown, PlatformWebhookListData>('/webhook/platform/list', { params: { repo_key } }),
  deletePlatformWebhook: (repo_key: string, webhook_id: number) =>
    http.post<unknown, void>('/webhook/platform/delete', null, { params: { repo_key, webhook_id } }),
}

export const logApi = {
  listOperations: (params?: OperationLogParams) =>
    http.get<unknown, OperationLogListData>('/logs/operations', { params }),
  listSync: (params?: SyncLogParams) =>
    http.get<unknown, SyncLogListData>('/logs/sync', { params }),
  listSystem: (params?: SystemLogParams) =>
    http.get<unknown, SystemLogListData>('/logs/system', { params }),
}

export const systemApi = {
  status: () => http.get<unknown, SystemStatusData>('/system/status'),
  health: () => http.get<unknown, HealthData>('/system/health'),
}

// 统一错误类型,供业务层 catch 后判断
export { ApiError } from './http'

// 兼容旧导入(Dashboard 等页面仍在使用 SystemStatusResp 别名)
export type { SystemStatusData as SystemStatusResp , PlatformWebhookData, PlatformWebhookListData } from '@/types/api'

export default http
