<template>
  <a-layout-header class="app-header">
    <div class="header-left">
      <a-button type="text" class="collapse-btn" @click="$emit('toggle')">
        <MenuUnfoldOutlined v-if="collapsed" />
        <MenuFoldOutlined v-else />
      </a-button>
      <a-breadcrumb class="header-crumb">
        <a-breadcrumb-item>
          <router-link to="/dashboard">GitFerry</router-link>
        </a-breadcrumb-item>
        <a-breadcrumb-item v-if="parentTitle && parentTitle !== currentTitle">
          {{ parentTitle }}
        </a-breadcrumb-item>
        <a-breadcrumb-item>{{ currentTitle }}</a-breadcrumb-item>
      </a-breadcrumb>
    </div>

    <div class="header-right">
      <a-tooltip v-if="showAI" title="AI 运维助手">
        <a-button type="text" class="icon-btn" @click="toggleAssistant">
          <RobotOutlined />
        </a-button>
      </a-tooltip>
      <a-divider type="vertical" class="header-divider" />
      <a-dropdown placement="bottomRight">
        <span class="user-trigger" @click.prevent>
          <a-avatar :size="26" class="user-avatar">
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
import { computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  UserOutlined,
  DownOutlined,
  LogoutOutlined,
  RobotOutlined,
} from '@ant-design/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { useAIChat } from '@/composables/useAIChat'

defineProps<{ collapsed: boolean }>()
defineEmits<{ (e: 'toggle'): void }>()

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const ai = useAIChat()
// 探测一次 AI 可用性,未启用则顶栏不展示入口
onMounted(() => {
  if (ai.enabled.value === null) void ai.loadStatus()
})

// 仅在 AI 启用时露出入口(false / 探测中均隐藏)
const showAI = computed(() => ai.enabled.value === true)

const currentTitle = computed(() => (route.meta.title as string) || 'GitFerry')

// 面包屑父级：与侧栏分组对齐
const parentTitle = computed(() => {
  const p = route.path
  if (p.startsWith('/sync')) return '同步任务'
  if (p.startsWith('/repos') || p.startsWith('/local-repos/')) return '仓库管理'
  if (p.startsWith('/mirror')) return '镜像中心'
  if (p.startsWith('/webhook')) return 'Webhook 规则'
  if (p.startsWith('/logs/webhook-events')) return 'Webhook 事件'
  if (p.startsWith('/logs')) return '日志'
  if (p.startsWith('/settings/ai')) return 'AI 助手'
  if (p.startsWith('/settings')) return '系统'
  return ''
})

const maskedKey = computed(() => {
  const k = authStore.getApiKey() || ''
  if (!k) return ''
  if (k.length <= 8) return '****'
  return `${k.slice(0, 4)}****${k.slice(-4)}`
})

function toggleAssistant() {
  ai.toggle()
}

function handleLogout() {
  // AI 会话含业务上下文,换账号登录不应延续上一账号的对话
  ai.reset()
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
  padding: 0 16px 0 8px;
  background: $bg-primary;
  border-bottom: 1px solid $border-muted;
  z-index: 10;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.collapse-btn {
  font-size: 16px;
  width: 32px;
  height: 32px;
  color: $text-secondary;

  &:hover {
    color: $text-primary;
    background: $bg-hover;
  }
}

.header-crumb {
  :deep(.ant-breadcrumb-link) {
    font-size: $fs-body;
  }
}

.header-right {
  display: flex;
  align-items: center;
  gap: 4px;
}

.icon-btn {
  width: 32px;
  height: 32px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: $text-secondary;
  border-radius: $radius-md;

  &:hover {
    color: $primary;
    background: $primary-soft;
  }
}

.header-divider {
  height: 16px;
  margin: 0 8px;
}

.user-trigger {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 4px 8px;
  border-radius: $radius-md;
  cursor: pointer;
  transition: background $duration-fast $ease;

  &:hover {
    background: $bg-hover;
  }
}

.user-avatar {
  background: $primary;
  flex-shrink: 0;
}

.user-key {
  font-size: 12px;
  color: $text-secondary;
  font-family: $font-mono;
  max-width: 140px;
}

.user-caret {
  font-size: 10px;
  color: $text-tertiary;
}
</style>
