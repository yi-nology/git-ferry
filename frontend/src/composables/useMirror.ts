import { useQuery, useMutation, useQueryClient } from '@tanstack/vue-query'
import type { Ref } from 'vue'
import { mirrorApi } from '@/api/mirror'
import type { CreateMirrorChannelInput, MirrorTargetInput } from '@/api/mirror'

export const MIRROR_CHANNELS_KEY = 'mirror-channels'
export const MIRROR_CHANNEL_KEY = 'mirror-channel'
export const MIRROR_VERSIONS_KEY = 'mirror-versions'
export const MIRROR_RUNS_KEY = 'mirror-runs'
export const MIRROR_RUN_KEY = 'mirror-run'

const RUNNING_STATES = ['pending', 'running']

/** 通道列表 */
export function useMirrorChannelsQuery() {
  return useQuery({
    queryKey: [MIRROR_CHANNELS_KEY],
    queryFn: () => mirrorApi.listChannels({ page: 1, pageSize: 100 }),
    select: (data) => data.list || [],
  })
}

/** 通道详情 */
export function useMirrorChannelQuery(id: Ref<number>) {
  return useQuery({
    queryKey: [MIRROR_CHANNEL_KEY, id],
    queryFn: () => mirrorApi.getChannel(id.value),
    enabled: () => id.value > 0,
  })
}

/** 版本矩阵;有执行中的 run 时 2s 轮询 */
export function useMirrorVersionsQuery(id: Ref<number>, poll: Ref<boolean>) {
  return useQuery({
    queryKey: [MIRROR_VERSIONS_KEY, id],
    queryFn: () => mirrorApi.versions(id.value),
    enabled: () => id.value > 0,
    refetchInterval: () => (poll.value ? 2000 : false),
  })
}

/** 执行记录列表 */
export function useMirrorRunsQuery(id: Ref<number>) {
  return useQuery({
    queryKey: [MIRROR_RUNS_KEY, id],
    queryFn: () => mirrorApi.listRuns(id.value, { page: 1, pageSize: 50 }),
    enabled: () => id.value > 0,
  })
}

/** 单条执行记录;进行中时 2s 轮询 */
export function useMirrorRunQuery(runId: Ref<number>) {
  return useQuery({
    queryKey: [MIRROR_RUN_KEY, runId],
    queryFn: () => mirrorApi.getRun(runId.value),
    enabled: () => runId.value > 0,
    refetchInterval: (query) => {
      const status = query.state.data?.status
      return status && RUNNING_STATES.includes(status) ? 2000 : false
    },
  })
}

export function useCreateMirrorChannelMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: CreateMirrorChannelInput) => mirrorApi.createChannel(data),
    onSuccess: () => qc.invalidateQueries({ queryKey: [MIRROR_CHANNELS_KEY] }),
  })
}

export function useDeleteMirrorChannelMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, confirm }: { id: number; confirm: string }) =>
      mirrorApi.deleteChannel(id, confirm),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [MIRROR_CHANNELS_KEY] })
      qc.invalidateQueries({ queryKey: [MIRROR_VERSIONS_KEY] })
    },
  })
}

export function useTestMirrorTargetMutation() {
  return useMutation({ mutationFn: (targetId: number) => mirrorApi.testTarget(targetId) })
}

export function useMirrorPreviewMutation() {
  return useMutation({
    mutationFn: ({ channelId, targetId, tag }: { channelId: number; targetId: number; tag: string }) =>
      mirrorApi.preview(channelId, { targetId, tag }),
  })
}

export function useExecuteMirrorRunMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: {
      channelId: number
      targetId: number
      tags: string[]
      allowOverwrite: boolean
    }) =>
      mirrorApi.createRun(req.channelId, {
        targetId: req.targetId,
        tags: req.tags,
        allowOverwrite: req.allowOverwrite,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [MIRROR_RUNS_KEY] })
    },
  })
}

export function useMirrorVerifyMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ channelId, targetId, tag }: { channelId: number; targetId: number; tag: string }) =>
      mirrorApi.verify(channelId, { targetId, tag }),
    onSuccess: () => qc.invalidateQueries({ queryKey: [MIRROR_VERSIONS_KEY] }),
  })
}

export function useDeleteMirrorTargetMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (targetId: number) => mirrorApi.deleteTarget(targetId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [MIRROR_CHANNEL_KEY] })
      qc.invalidateQueries({ queryKey: [MIRROR_CHANNELS_KEY] })
    },
  })
}
