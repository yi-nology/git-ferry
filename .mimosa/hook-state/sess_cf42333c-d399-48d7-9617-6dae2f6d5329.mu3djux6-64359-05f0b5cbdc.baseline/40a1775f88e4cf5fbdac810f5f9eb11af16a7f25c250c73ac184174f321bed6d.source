import { useQuery, useMutation, useQueryClient } from '@tanstack/vue-query'
import { webhookApi } from '@/api'
import type { WebhookRule, CreateRuleRequest, UpdateRuleRequest } from '@/types'

export const RULES_QUERY_KEY = 'webhook-rules'
export const EVENTS_QUERY_KEY = 'webhook-events'

/** Webhook 规则列表查询 */
export function useWebhookRulesQuery(repoKey: string) {
  return useQuery({
    queryKey: [RULES_QUERY_KEY, repoKey],
    queryFn: () => webhookApi.listRules(repoKey),
    select: (data) => (data.rules || []) as WebhookRule[],
    enabled: !!repoKey,
  })
}

/** Webhook 事件列表查询 */
export function useWebhookEventsQuery(repoKey: string, limit = 50) {
  return useQuery({
    queryKey: [EVENTS_QUERY_KEY, repoKey, limit],
    queryFn: () => webhookApi.listEvents({ repo_key: repoKey, limit }),
    select: (data) => data.events || [],
    enabled: !!repoKey,
  })
}

/** 创建规则 mutation */
export function useCreateRuleMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: CreateRuleRequest) => webhookApi.createRule(req),
    onSuccess: () => qc.invalidateQueries({ queryKey: [RULES_QUERY_KEY] }),
  })
}

/** 更新规则 mutation */
export function useUpdateRuleMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: UpdateRuleRequest) => webhookApi.updateRule(req),
    onSuccess: () => qc.invalidateQueries({ queryKey: [RULES_QUERY_KEY] }),
  })
}

/** 删除规则 mutation */
export function useDeleteRuleMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => webhookApi.deleteRule(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: [RULES_QUERY_KEY] }),
  })
}

/** 重试事件 mutation */
export function useRetryEventMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => webhookApi.retryEvent(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: [EVENTS_QUERY_KEY] }),
  })
}
