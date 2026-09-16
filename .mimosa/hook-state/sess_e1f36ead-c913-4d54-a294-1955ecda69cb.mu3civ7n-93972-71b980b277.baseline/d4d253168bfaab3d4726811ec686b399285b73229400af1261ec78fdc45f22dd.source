import api from './index'

export interface Platform {
  id: number
  key: string
  name: string
  type: string
  instance_url: string
  instanceUrl: string  // camelCase from backend
  api_url: string
  apiUrl: string  // camelCase from backend
  access_token?: string
  has_token?: boolean  // 后端不回传令牌明文,仅回传是否已配置
  skip_tls_verify: boolean
  skipTlsVerify: boolean  // camelCase from backend
  ca_cert_path: string
  caCertPath: string  // camelCase from backend
  proxy_url: string
  proxyUrl: string  // camelCase from backend
  is_default: boolean
  isDefault: boolean  // camelCase from backend
  status: string
  repo_count: number
  repoCount: number  // camelCase from backend
  last_test_at?: string
  lastTestAt?: string  // camelCase from backend
  last_test_result?: string
  lastTestResult?: string  // camelCase from backend
  created_at: string
  createdAt: string  // camelCase from backend
  updated_at: string
  updatedAt: string  // camelCase from backend
}

export interface CreatePlatformRequest {
  name: string
  type: string
  instance_url?: string
  api_url: string
  access_token: string
  skip_tls_verify?: boolean
  ca_cert_path?: string
  proxy_url?: string
  is_default?: boolean
}

export interface UpdatePlatformRequest {
  key: string
  name?: string
  instance_url?: string
  api_url?: string
  access_token?: string
  skip_tls_verify?: boolean
  ca_cert_path?: string
  proxy_url?: string
  is_default?: boolean
}

export const platformApi = {
  // 列出所有平台
  list: () =>
    api.get<any, { platforms: Platform[]; total: number }>('/platforms'),

  // 获取单个平台
  get: (key: string) =>
    api.get<any, { platform: Platform }>('/platform', { params: { key } }),

  // 创建平台
  create: (data: CreatePlatformRequest) =>
    api.post<any, { platform: Platform }>('/platform/create', data),

  // 更新平台
  update: (data: UpdatePlatformRequest) =>
    api.post<any, { platform: Platform }>('/platform/update', data),

  // 删除平台
  delete: (key: string) =>
    api.post<any, void>('/platform/delete', null, { params: { key } }),

  // 测试连接
  test: (key: string) =>
    api.post<any, { result: any }>('/platform/test', null, { params: { key } }),

  // 设置默认
  setDefault: (key: string) =>
    api.post<any, { message: string }>('/platform/set-default', null, { params: { key } }),

  // 列出平台上的仓库(远程分页;has_more 提示是否还有下一页)
  listRepos: (key: string, page = 1, perPage = 20) =>
    api.get<any, { repos: any[]; total: number; has_more: boolean }>(
      '/platform/repos',
      { params: { key, page, per_page: perPage } },
    ),

  // 同步仓库到本地
  syncRepos: (key: string) =>
    api.post<any, { message: string; synced_count: number }>('/platform/sync-repos', null, { params: { key } }),
}
