import { defineStore } from 'pinia'
import { ref } from 'vue'
import { webhookApi } from '@/api'
import type { WebhookRule, WebhookEvent, CreateRuleRequest, UpdateRuleRequest } from '@/types'

export const useWebhookStore = defineStore('webhook', () => {
  const rules = ref<WebhookRule[]>([])
  const events = ref<WebhookEvent[]>([])
  const loading = ref(false)

  async function fetchRules(repoKey: string) {
    loading.value = true
    try {
      const data = await webhookApi.listRules(repoKey)
      rules.value = data.rules || []
    } finally {
      loading.value = false
    }
  }

  async function getRule(id: number): Promise<WebhookRule> {
    const data = await webhookApi.getRule(id)
    return data.rule
  }

  async function createRule(req: CreateRuleRequest): Promise<WebhookRule> {
    const data = await webhookApi.createRule(req)
    return data.rule
  }

  async function updateRule(req: UpdateRuleRequest): Promise<WebhookRule> {
    const data = await webhookApi.updateRule(req)
    return data.rule
  }

  async function deleteRule(id: number) {
    await webhookApi.deleteRule(id)
  }

  async function fetchEvents(repoKey: string, limit = 50) {
    loading.value = true
    try {
      const data = await webhookApi.listEvents({ repo_key: repoKey, limit })
      events.value = data.events || []
    } finally {
      loading.value = false
    }
  }

  async function retryEvent(id: number) {
    await webhookApi.retryEvent(id)
  }

  return {
    rules, events, loading,
    fetchRules, getRule, createRule, updateRule, deleteRule,
    fetchEvents, retryEvent,
  }
})
