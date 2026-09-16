import { defineStore } from 'pinia'
import { ref } from 'vue'
import { syncTaskApi, type TaskListParams } from '@/api'
import type { SyncTask, SyncRun, CreateTaskRequest, UpdateTaskRequest } from '@/types'

export type { TaskListParams }

export const useSyncTaskStore = defineStore('syncTask', () => {
  const tasks = ref<SyncTask[]>([])
  const total = ref(0)
  const loading = ref(false)
  const history = ref<SyncRun[]>([])
  // 记忆上次查询参数,便于刷新时复用
  const lastParams = ref<TaskListParams>({})

  async function fetchTasks(params?: TaskListParams) {
    loading.value = true
    if (params) lastParams.value = { ...params }
    try {
      const data = await syncTaskApi.list(params)
      tasks.value = data.tasks
      total.value = data.total ?? 0
    } finally {
      loading.value = false
    }
  }

  async function refreshTasks() {
    await fetchTasks(lastParams.value)
  }

  async function getTask(key: string): Promise<SyncTask> {
    const data = await syncTaskApi.get(key)
    return data.task
  }

  async function createTask(req: CreateTaskRequest): Promise<SyncTask> {
    const data = await syncTaskApi.create(req)
    // 乐观更新:插入本地列表
    tasks.value.unshift(data.task)
    total.value++
    return data.task
  }

  async function updateTask(req: UpdateTaskRequest): Promise<SyncTask> {
    const data = await syncTaskApi.update(req)
    // 乐观更新:替换本地列表中的对应项
    const idx = tasks.value.findIndex((t) => t.key === req.key)
    if (idx !== -1) tasks.value[idx] = data.task
    return data.task
  }

  async function deleteTask(key: string) {
    await syncTaskApi.delete(key)
    // 乐观更新:从本地列表移除
    tasks.value = tasks.value.filter((t) => t.key !== key)
    total.value = Math.max(0, total.value - 1)
  }

  async function runTask(key: string) {
    await syncTaskApi.run(key)
  }

  async function fetchHistory(taskKey: string, limit = 50): Promise<SyncRun[]> {
    const data = await syncTaskApi.history({ task_key: taskKey, limit })
    const runs = data.runs || []
    history.value = runs
    return runs
  }

  /** 批量删除(后端无批量接口,逐个调用) */
  async function batchDelete(keys: string[]): Promise<{ succeeded: number; failed: number }> {
    const results = await Promise.allSettled(keys.map((k) => syncTaskApi.delete(k)))
    const succeeded = results.filter((r) => r.status === 'fulfilled').length
    const failed = results.length - succeeded
    // 乐观更新:从本地列表移除已删除项
    const keySet = new Set(keys)
    tasks.value = tasks.value.filter((t) => !keySet.has(t.key))
    total.value = Math.max(0, total.value - succeeded)
    return { succeeded, failed }
  }

  return {
    tasks, total, loading, history, lastParams,
    fetchTasks, refreshTasks, getTask, createTask, updateTask, deleteTask, runTask, fetchHistory, batchDelete,
  }
})
