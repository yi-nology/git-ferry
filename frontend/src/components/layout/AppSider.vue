<template>
  <a-layout-sider
    class="app-sider"
    :collapsed="collapsed"
    :trigger="null"
    :width="220"
    :collapsed-width="64"
  >
    <!-- Logo -->
    <div class="sider-logo" @click="router.push('/dashboard')">
      <div class="logo-icon">
        <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10" />
          <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" />
        </svg>
      </div>
      <span v-if="!collapsed" class="logo-text">GitFerry</span>
    </div>

    <a-menu
      :selected-keys="selectedKeys"
      v-model:openKeys="openKeys"
      mode="inline"
      theme="light"
      @click="onMenuClick"
    >
      <a-menu-item key="/dashboard">
        <template #icon><DashboardOutlined /></template>
        <span>仪表盘</span>
      </a-menu-item>

      <a-sub-menu key="sync">
        <template #icon><SyncOutlined /></template>
        <template #title>同步管理</template>
        <a-menu-item key="/sync">任务列表</a-menu-item>
        <a-menu-item key="/sync/records">执行记录</a-menu-item>
      </a-sub-menu>

      <a-menu-item key="/repos">
        <template #icon><FolderOutlined /></template>
        <span>仓库管理</span>
      </a-menu-item>

      <a-menu-item key="/mirror">
        <template #icon><CloudUploadOutlined /></template>
        <span>镜像中心</span>
      </a-menu-item>

      <a-menu-item key="/webhook/rules">
        <template #icon><ApiOutlined /></template>
        <span>Webhook 规则</span>
      </a-menu-item>

      <a-sub-menu key="logs">
        <template #icon><FileTextOutlined /></template>
        <template #title>日志中心</template>
        <a-menu-item key="/logs/operations">操作日志</a-menu-item>
        <a-menu-item key="/logs/webhook-events">Webhook 事件</a-menu-item>
      </a-sub-menu>

      <a-menu-item key="/settings/platforms">
        <template #icon><SettingOutlined /></template>
        <span>平台管理</span>
      </a-menu-item>
    </a-menu>
  </a-layout-sider>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  DashboardOutlined,
  SyncOutlined,
  ApiOutlined,
  FolderOutlined,
  SettingOutlined,
  FileTextOutlined,
  CloudUploadOutlined,
} from '@ant-design/icons-vue'

defineProps<{ collapsed: boolean }>()

const route = useRoute()
const router = useRouter()

// 动态详情页高亮父级
const activeKey = computed(() => {
  const p = route.path
  if (p.startsWith('/repos/config/') || p.startsWith('/local-repos/')) return '/repos'
  return p
})
const selectedKeys = computed(() => [activeKey.value])

// 子菜单所属映射(用于自动展开当前路由所在分组)
const SUBMENU_OF: Record<string, string> = {
  '/sync': 'sync',
  '/sync/records': 'sync',
  '/logs/operations': 'logs',
  '/logs/webhook-events': 'logs',
}

const openKeys = ref<string[]>([])
watch(
  () => activeKey.value,
  (key) => {
    const sub = SUBMENU_OF[key]
    if (sub && !openKeys.value.includes(sub)) {
      openKeys.value = [...openKeys.value, sub]
    }
  },
  { immediate: true },
)

function onMenuClick({ key }: { key: string }) {
  if (key !== route.path) router.push(key)
}
</script>

<style scoped lang="scss">
@use '@/styles/variables.scss' as *;

.app-sider {
  background: $bg-primary;
  box-shadow: 2px 0 8px 0 rgba(0, 0, 0, 0.04);
  z-index: 20;
}

.sider-logo {
  display: flex;
  align-items: center;
  gap: 10px;
  height: $header-height;
  padding: 0 18px;
  cursor: pointer;
  overflow: hidden;
  border-bottom: 1px solid $border;

  .logo-icon {
    color: $primary;
    display: flex;
    align-items: center;
    flex-shrink: 0;
  }

  .logo-text {
    color: $primary;
    font-size: 18px;
    font-weight: 700;
    white-space: nowrap;
    letter-spacing: -0.5px;
  }
}

:deep(.ant-menu) {
  border-inline-end: none !important;
}
</style>
