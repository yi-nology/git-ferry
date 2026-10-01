<template>
  <a-layout-sider
    class="app-sider"
    :collapsed="collapsed"
    :trigger="null"
    :width="232"
    :collapsed-width="64"
  >
    <!-- Logo -->
    <div class="sider-logo" @click="router.push('/dashboard')">
      <div class="logo-icon">
        <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10" />
          <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" />
        </svg>
      </div>
      <div v-if="!collapsed" class="logo-texts">
        <span class="logo-text">GitFerry</span>
        <span class="logo-tag">代码摆渡</span>
      </div>
    </div>

    <a-menu
      :selected-keys="selectedKeys"
      mode="inline"
      theme="light"
      class="sider-menu"
      @click="onMenuClick"
    >
      <a-menu-item key="/dashboard">
        <template #icon><DashboardOutlined /></template>
        <span>仪表盘</span>
      </a-menu-item>

      <a-menu-item-group key="g-manage">
        <template #title>
          <span v-if="!collapsed" class="group-title">管理</span>
        </template>
        <a-menu-item key="/sync">
          <template #icon><SyncOutlined /></template>
          <span>同步任务</span>
        </a-menu-item>
        <a-menu-item key="/sync/records">
          <template #icon><HistoryOutlined /></template>
          <span>执行记录</span>
        </a-menu-item>
        <a-menu-item key="/repos">
          <template #icon><FolderOutlined /></template>
          <span>仓库管理</span>
        </a-menu-item>
        <a-menu-item key="/mirror">
          <template #icon><CloudUploadOutlined /></template>
          <span>镜像中心</span>
        </a-menu-item>
      </a-menu-item-group>

      <a-menu-item-group key="g-auto">
        <template #title>
          <span v-if="!collapsed" class="group-title">自动化</span>
        </template>
        <a-menu-item key="/webhook/rules">
          <template #icon><ApiOutlined /></template>
          <span>Webhook 规则</span>
        </a-menu-item>
        <a-menu-item key="/logs/webhook-events">
          <template #icon><ThunderboltOutlined /></template>
          <span>Webhook 事件</span>
        </a-menu-item>
      </a-menu-item-group>

      <a-menu-item-group key="g-ops">
        <template #title>
          <span v-if="!collapsed" class="group-title">运维</span>
        </template>
        <a-menu-item key="/ops">
          <template #icon><ToolOutlined /></template>
          <span>运维中心</span>
        </a-menu-item>
        <a-menu-item key="/logs/operations">
          <template #icon><FileTextOutlined /></template>
          <span>操作日志</span>
        </a-menu-item>
      </a-menu-item-group>

      <a-menu-item-group key="g-sys">
        <template #title>
          <span v-if="!collapsed" class="group-title">系统</span>
        </template>
        <a-menu-item key="/settings/ai">
          <template #icon><RobotOutlined /></template>
          <span>AI 助手</span>
        </a-menu-item>
        <a-menu-item key="/settings/dev">
          <template #icon><CodeOutlined /></template>
          <span>CLI / Agent</span>
        </a-menu-item>
        <a-menu-item key="/settings/platforms">
          <template #icon><SettingOutlined /></template>
          <span>平台管理</span>
        </a-menu-item>
      </a-menu-item-group>
    </a-menu>
  </a-layout-sider>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  DashboardOutlined,
  ToolOutlined,
  SyncOutlined,
  ApiOutlined,
  FolderOutlined,
  SettingOutlined,
  FileTextOutlined,
  CloudUploadOutlined,
  HistoryOutlined,
  ThunderboltOutlined,
  RobotOutlined,
  CodeOutlined,
} from '@ant-design/icons-vue'

defineProps<{ collapsed: boolean }>()

const route = useRoute()
const router = useRouter()

// 动态详情页高亮父级
const activeKey = computed(() => {
  const p = route.path
  if (p.startsWith('/repos/config/') || p.startsWith('/local-repos/')) return '/repos'
  if (p.startsWith('/mirror/')) return '/mirror'
  return p
})
const selectedKeys = computed(() => [activeKey.value])

function onMenuClick({ key }: { key: string }) {
  if (key !== route.path) router.push(key)
}
</script>

<style scoped lang="scss">
@use '@/styles/variables.scss' as *;

.app-sider {
  background: $bg-primary;
  border-right: 1px solid $border-light;
  z-index: 20;
  overflow: hidden;
}

.sider-logo {
  display: flex;
  align-items: center;
  gap: 10px;
  height: $header-height;
  padding: 0 16px;
  cursor: pointer;
  overflow: hidden;
  border-bottom: 1px solid $border-muted;
  flex-shrink: 0;

  .logo-icon {
    color: $primary;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }

  .logo-texts {
    display: flex;
    flex-direction: column;
    line-height: 1.15;
    overflow: hidden;
  }

  .logo-text {
    color: $text-primary;
    font-size: 15px;
    font-weight: 700;
    white-space: nowrap;
    letter-spacing: -0.3px;
  }

  .logo-tag {
    color: $text-tertiary;
    font-size: 11px;
    white-space: nowrap;
    margin-top: 1px;
  }
}

.sider-menu {
  border-inline-end: none !important;
  padding: 12px 0 24px;
  background: transparent;
}

.group-title {
  display: block;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.6px;
  color: $text-tertiary;
  padding: 12px 8px 4px;
  text-transform: none;
}

:deep(.ant-menu-item-group) {
  margin-bottom: 4px;

  .ant-menu-item-group-title {
    padding: 0;
    line-height: 0;
    height: 0;
    overflow: hidden;
  }
}

:deep(.ant-menu-item) {
  margin-block: 2px;
  width: calc(100% - 16px);
}

:deep(.ant-menu-item-selected) {
  font-weight: 600;
}
</style>
