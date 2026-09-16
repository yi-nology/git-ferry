import { defineStore } from 'pinia'
import { computed } from 'vue'
import { useLocalStorage } from '@vueuse/core'

export const useAuthStore = defineStore('auth', () => {
  // useLocalStorage 自动同步 localStorage、响应式、跨 tab 同步
  const apiKey = useLocalStorage<string>('git-sync-api-key', '')

  const isAuthenticated = computed(() => !!apiKey.value)

  function setApiKey(key: string) {
    apiKey.value = key
  }

  function clearApiKey() {
    apiKey.value = ''
  }

  function getApiKey() {
    return apiKey.value
  }

  return {
    apiKey,
    isAuthenticated,
    setApiKey,
    clearApiKey,
    getApiKey,
  }
})
