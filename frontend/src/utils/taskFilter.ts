import type { SyncTask } from '@/types'

/** Ant Select 远程/本地过滤:按任务名称或 key 模糊匹配 */
export function makeTaskFilter(tasks: () => SyncTask[]) {
  return (input: string, option: { value?: string }): boolean => {
    const task = tasks().find((t) => t.key === option.value)
    if (!task) return false
    const search = input.toLowerCase()
    return task.name.toLowerCase().includes(search) || task.key.toLowerCase().includes(search)
  }
}
