import { useQuery, useMutation, useQueryClient } from '@tanstack/vue-query'
import { repoApi, type RepoQueryParams } from '@/api'
import type { Repo, CreateRepoRequest, UpdateRepoRequest } from '@/types'

export const REPO_QUERY_KEY = 'repos'

/** 仓库列表查询(带缓存、去重、窗口聚焦刷新) */
export function useReposQuery(params?: RepoQueryParams) {
  const query = { page: 1, page_size: 500, ...params }
  return useQuery({
    queryKey: [REPO_QUERY_KEY, query],
    queryFn: () => repoApi.list(query),
    select: (data) => ({
      repos: data.list as Repo[],
      total: (data.pagination?.total ?? 0) as number,
    }),
  })
}

/** 创建仓库 mutation(乐观更新) */
export function useCreateRepoMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: CreateRepoRequest) => repoApi.create(req),
    onSuccess: () => qc.invalidateQueries({ queryKey: [REPO_QUERY_KEY] }),
  })
}

/** 更新仓库 mutation(乐观更新) */
export function useUpdateRepoMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: UpdateRepoRequest) => repoApi.update(req),
    onSuccess: () => qc.invalidateQueries({ queryKey: [REPO_QUERY_KEY] }),
  })
}

/** 删除仓库 mutation */
export function useDeleteRepoMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (key: string) => repoApi.delete(key),
    onSuccess: () => qc.invalidateQueries({ queryKey: [REPO_QUERY_KEY] }),
  })
}

/** 批量删除 mutation */
export function useBatchDeleteReposMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (keys: string[]) => repoApi.batchDelete(keys),
    onSuccess: () => qc.invalidateQueries({ queryKey: [REPO_QUERY_KEY] }),
  })
}
