/** 平台类型预设:名称/默认实例/API 路径/文案提示 */
export interface PlatformPreset {
  name: string
  defaultInstance: string
  apiPath: string
  urlTip: string
  tokenTip: string
  instancePlaceholder: string
}

export const PLATFORM_PRESETS: Record<string, PlatformPreset> = {
  github: {
    name: 'GitHub',
    defaultInstance: 'github.com',
    apiPath: '/api/v3',
    urlTip: '企业版地址格式: https://github.company.com/api/v3',
    tokenTip: 'GitHub Personal Access Token (需要 repo 权限)',
    instancePlaceholder: 'github.com (默认) 或 github.company.com',
  },
  gitlab: {
    name: 'GitLab',
    defaultInstance: 'gitlab.com',
    apiPath: '/api/v4',
    urlTip: '自建地址格式: https://gitlab.company.com/api/v4',
    tokenTip: 'GitLab Personal Access Token (需要 read_repository 权限)',
    instancePlaceholder: 'gitlab.com (默认) 或 gitlab.company.com',
  },
  gitea: {
    name: 'Gitea',
    defaultInstance: 'gitea.com',
    apiPath: '/api/v1',
    urlTip: '自建地址格式: https://gitea.company.com/api/v1',
    tokenTip: 'Gitea Access Token (需要 repo 权限)',
    instancePlaceholder: 'gitea.com (默认) 或 gitea.company.com',
  },
  gitee: {
    name: 'Gitee',
    defaultInstance: 'gitee.com',
    apiPath: '/api/v5',
    urlTip: '默认 API 地址',
    tokenTip: 'Gitee 私人令牌 (需要 projects 权限)',
    instancePlaceholder: 'gitee.com',
  },
  gitcode: {
    name: 'GitCode',
    defaultInstance: 'gitcode.com',
    apiPath: '/api/v5',
    urlTip: '默认 API 地址',
    tokenTip: 'GitCode Access Token (需要 repo 权限)',
    instancePlaceholder: 'gitcode.com',
  },
  atomgit: {
    name: 'AtomGit',
    defaultInstance: 'atomgit.com',
    apiPath: '/api/v1',
    urlTip: '默认 API 地址',
    tokenTip: 'AtomGit Access Token (需要 repo 权限)',
    instancePlaceholder: 'atomgit.com',
  },
  tencent_code: {
    name: '腾讯工蜂',
    defaultInstance: 'git.code.tencent.com',
    apiPath: '/api/v3',
    urlTip: '默认 API 地址',
    tokenTip: '腾讯工蜂 Access Token (需要 repo 权限)',
    instancePlaceholder: 'git.code.tencent.com',
  },
  custom: {
    name: '自定义平台',
    defaultInstance: '',
    apiPath: '',
    urlTip: '请输入完整的 API 地址',
    tokenTip: '平台对应的访问令牌',
    instancePlaceholder: '',
  },
}
