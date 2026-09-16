import http from './http'

// ===== 类型 =====

export interface MirrorTarget {
  id: number
  channelId: number
  remote: string
  repoUrl: string
  targetModule: string
  credType: string
  username?: string
  hasCredential: boolean
}

export interface MirrorChannel {
  id: number
  name: string
  mode: 'publish' | 'backup'
  repoKey: string
  module?: string
  targets?: MirrorTarget[]
  createdAt?: string
}

export interface MirrorTargetInput {
  remote: string
  repoUrl: string
  targetModule: string
  credType: string
  credential?: string
  username?: string
}

export interface CreateMirrorChannelInput {
  name: string
  mode: string
  repoKey: string
  targets: MirrorTargetInput[]
}

export interface MirrorTagStatus {
  tag: string
  status: string
  detail?: string
  commit?: string
  tree?: string
}

export interface MirrorStep {
  name: string
  status: string
  detail?: string
}

export interface MirrorRun {
  id: number
  channelId: number
  targetId: number
  kind: string
  tags: string
  status: string
  tagStatuses: string
  steps: string
  report: string
  error?: string
  allowOverwrite: boolean
  startedAt?: string
  finishedAt?: string
  durationMs: number
}

export interface MirrorVersionTarget {
  targetId: number
  target: string
  state: string
  executedAt?: string
  commit?: string
  tree?: string
  verify: string
}

export interface MirrorVersion {
  tag: string
  commit?: string
  targets: MirrorVersionTarget[]
}

export interface MirrorVersionsResult {
  mode: string
  module: string
  versions: MirrorVersion[]
}

export interface MirrorPreviewReport {
  tag: string
  sourceCommit: string
  tree: string
  commit: string
  replacedFiles: string[]
  verifyCmd: string
  remoteTagExisted: boolean
  treeMatchedRemote: boolean
  overwroteDivergent: boolean
}

// ===== API =====

export const mirrorApi = {
  listChannels: (params?: { page?: number; pageSize?: number }) =>
    http.get<unknown, { list: MirrorChannel[]; total: number }>('/mirror/channels', { params }),
  createChannel: (data: CreateMirrorChannelInput) =>
    http.post<unknown, MirrorChannel>('/mirror/channels', data),
  getChannel: (id: number) => http.get<unknown, MirrorChannel>(`/mirror/channels/${id}`),
  deleteChannel: (id: number, confirm: string) =>
    http.post<unknown, unknown>(`/mirror/channels/${id}/delete`, null, { params: { confirm } }),

  versions: (id: number) => http.get<unknown, MirrorVersionsResult>(`/mirror/channels/${id}/versions`),
  preview: (id: number, data: { targetId: number; tag: string }) =>
    http.post<unknown, MirrorPreviewReport>(`/mirror/channels/${id}/preview`, data),
  createRun: (id: number, data: { targetId: number; tags: string[]; allowOverwrite: boolean }) =>
    http.post<unknown, MirrorRun>(`/mirror/channels/${id}/runs`, data),
  listRuns: (id: number, params?: { page?: number; pageSize?: number }) =>
    http.get<unknown, { list: MirrorRun[]; total: number }>(`/mirror/channels/${id}/runs`, { params }),
  getRun: (id: number) => http.get<unknown, MirrorRun>(`/mirror/runs/${id}`),
  verify: (id: number, data: { targetId: number; tag: string }) =>
    http.post<unknown, MirrorRun>(`/mirror/channels/${id}/verify`, data),

  updateTarget: (id: number, data: MirrorTargetInput) =>
    http.post<unknown, MirrorTarget>(`/mirror/targets/${id}/update`, data),
  deleteTarget: (id: number) => http.post<unknown, unknown>(`/mirror/targets/${id}/delete`),
  testTarget: (id: number) => http.post<unknown, unknown>(`/mirror/targets/${id}/test`),
}
