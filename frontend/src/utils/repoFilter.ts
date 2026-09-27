import type { Repo } from '@/types'

/**
 * Ant Design Select 远程/本地过滤:按仓库名称或 key 模糊匹配。
 * 各列表页复用,避免复制粘贴。
 */
export function makeRepoFilter(repos: () => Repo[]) {
  return (input: string, option: { value?: string }): boolean => {
    const repo = repos().find((r) => r.key === option.value)
    if (!repo) return false
    const search = input.toLowerCase()
    return repo.name.toLowerCase().includes(search) || repo.key.toLowerCase().includes(search)
  }
}
