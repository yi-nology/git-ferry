import { useQuery, useMutation, useQueryClient } from '@tanstack/vue-query'
import { syncTaskApi, type TaskListParams } from '@/api'
import type { SyncTask, CreateTaskRequest, UpdateTaskRequest } from '@/types'

export const TASK_QUERY_KEY = 'tasks'
export const HISTORY_QUERY_KEY = 'history'

/** 任务列表查询 */
export function useTasksQuery(params?: TaskListParams) {
  return useQuery({
    queryKey: [TASK_QUERY_KEY, params],
    queryFn: () => syncTaskApi.list(params),
    select: (data) => ({
      tasks: data.tasks as SyncTask[],
      total: (data.total ?? 0) as number,
    }),
  })
}

/** 任务执行历史查询 */
export function useHistoryQuery(taskKey: string, limit = 50) {
  return useQuery({
    queryKey: [HISTORY_QUERY_KEY, taskKey, limit],
    queryFn: () => syncTaskApi.history({ task_key: taskKey, limit }),
    select: (data) => data.runs || [],
    enabled: !!taskKey,
  })
}

/** 创建任务 mutation */
export function useCreateTaskMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: CreateTaskRequest) => syncTaskApi.create(req),
    onSuccess: () => qc.invalidateQueries({ queryKey: [TASK_QUERY_KEY] }),
  })
}

/** 更新任务 mutation */
export function useUpdateTaskMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: UpdateTaskRequest) => syncTaskApi.update(req),
    onSuccess: () => qc.invalidateQueries({ queryKey: [TASK_QUERY_KEY] }),
  })
}

/** 删除任务 mutation */
export function useDeleteTaskMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (key: string) => syncTaskApi.delete(key),
    onSuccess: () => qc.invalidateQueries({ queryKey: [TASK_QUERY_KEY] }),
  })
}

/** 批量删除 mutation */
export function useBatchDeleteTasksMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (keys: string[]) => {
      const results = await Promise.allSettled(keys.map((k) => syncTaskApi.delete(k)))
      const succeeded = results.filter((r) => r.status === 'fulfilled').length
      return { succeeded, failed: results.length - succeeded }
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: [TASK_QUERY_KEY] }),
  })
}

/** 运行任务 mutation */
export function useRunTaskMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (key: string) => syncTaskApi.run(key),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [TASK_QUERY_KEY] })
      qc.invalidateQueries({ queryKey: [HISTORY_QUERY_KEY] })
    },
  })
}
