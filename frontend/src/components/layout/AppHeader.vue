<template>
  <a-layout-header class="app-header">
    <div class="header-left">
      <a-button type="text" class="collapse-btn" @click="$emit('toggle')">
        <MenuUnfoldOutlined v-if="collapsed" />
        <MenuFoldOutlined v-else />
      </a-button>
      <a-breadcrumb>
        <a-breadcrumb-item>
          <router-link to="/dashboard">首页</router-link>
        </a-breadcrumb-item>
        <a-breadcrumb-item>{{ currentTitle }}</a-breadcrumb-item>
      </a-breadcrumb>
    </div>

    <div class="header-right">
      <a-dropdown placement="bottomRight">
        <span class="user-trigger" @click.prevent>
          <a-avatar :size="28" class="user-avatar">
            <template #icon><UserOutlined /></template>
          </a-avatar>
          <span v-if="maskedKey" class="user-key">{{ maskedKey }}</span>
          <DownOutlined class="user-caret" />
        </span>
        <template #overlay>
          <a-menu>
            <a-menu-item key="logout" @click="handleLogout">
              <LogoutOutlined />
              <span>退出登录</span>
            </a-menu-item>
          </a-menu>
        </template>
      </a-dropdown>
    </div>
  </a-layout-header>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  UserOutlined,
  DownOutlined,
  LogoutOutlined,
} from '@ant-design/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { useAIChat } from '@/composables/useAIChat'

defineProps<{ collapsed: boolean }>()
defineEmits<{ (e: 'toggle'): void }>()

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()

const currentTitle = computed(() => (route.meta.title as string) || 'GitFerry')

const maskedKey = computed(() => {
  const k = authStore.getApiKey() || ''
  if (!k) return ''
  if (k.length <= 8) return '****'
  return `${k.slice(0, 4)}****${k.slice(-4)}`
})

function handleLogout() {
  // AI 会话含业务上下文,换账号登录不应延续上一账号的对话
  useAIChat().reset()
  authStore.clearApiKey()
  router.push('/login')
}
</script>

<style scoped lang="scss">
@use '@/styles/variables.scss' as *;

.app-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: $header-height;
  padding: 0 20px;
  background: $bg-primary;
  border-bottom: 1px solid $border;
  box-shadow: 0 1px 4px 0 rgba(0, 0, 0, 0.04);
  z-index: 10;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.collapse-btn {
  font-size: 18px;
}

.header-right {
  display: flex;
  align-items: center;
}

.user-trigger {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 4px 8px;
  border-radius: $radius-sm;
  cursor: pointer;
  transition: background 0.2s;

  &:hover {
    background: $bg-secondary;
  }
}

.user-avatar {
  background: $primary;
}

.user-key {
  font-size: 13px;
  color: $text-primary;
  font-family: 'SFMono-Regular', Consolas, Menlo, monospace;
  max-width: 160px;
}

.user-caret {
  font-size: 10px;
  color: $text-secondary;
}
</style>
