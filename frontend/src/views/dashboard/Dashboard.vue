<template>
  <div class="page-container">
    <div class="page-header-bar">
      <div>
        <h1 class="page-title">仪表盘</h1>
        <p class="page-subtitle">系统运行概览</p>
      </div>
      <a-space>
        <a-button @click="router.push('/sync/new')">
          <template #icon><PlusOutlined /></template>
          新建同步任务
        </a-button>
        <a-button type="primary" @click="router.push('/repos')">
          <template #icon><FolderAddOutlined /></template>
          添加仓库
        </a-button>
      </a-space>
    </div>

    <!-- Stats Cards -->
    <div class="stats-row">
      <div
        v-for="card in statCards"
        :key="card.path"
        class="stat-card clickable"
        role="button"
        tabindex="0"
        @click="router.push(card.path)"
        @keydown.enter="router.push(card.path)"
      >
        <div class="stat-icon" :class="card.color"><component :is="card.icon" /></div>
        <div class="stat-content">
          <div class="stat-num">{{ card.value }}</div>
          <div class="stat-name">{{ card.label }}</div>
        </div>
        <div class="stat-arrow"><RightOutlined /></div>
      </div>
    </div>

    <!-- Quick Actions -->
    <div class="quick-actions">
      <div class="section-title">快捷操作</div>
      <div class="action-grid">
        <div
          v-for="act in quickActions"
          :key="act.path"
          class="action-item"
          role="button"
          tabindex="0"
          @click="router.push(act.path)"
          @keydown.enter="router.push(act.path)"
        >
          <div class="action-icon" :class="act.color"><component :is="act.icon" /></div>
          <span>{{ act.label }}</span>
        </div>
      </div>
    </div>

    <!-- Content Grid -->
    <div class="grid-row">
      <div class="content-card">
        <div class="card-header">
          <span class="card-title">
            <SyncOutlined style="margin-right: 8px; color: #1677ff;" />
            最近同步任务
          </span>
          <router-link to="/sync">
            <a-button type="link" size="small">查看全部 <RightOutlined /></a-button>
          </router-link>
        </div>
        <div class="card-body">
          <a-table
            :columns="taskColumns"
            :data-source="recentTasks"
            :pagination="false"
            size="small"
            row-key="key"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.dataIndex === 'name'">
                <a class="task-link" @click="router.push('/sync')">{{ record.name }}</a>
              </template>
              <template v-if="column.dataIndex === 'branch'">
                <span class="branch-tag">{{ record.source_branch }}</span>
                <ArrowRightOutlined style="margin: 0 4px; color: #bfbfbf; font-size: 12px;" />
                <span class="branch-tag">{{ record.target_branch }}</span>
              </template>
              <template v-if="column.dataIndex === 'last_status'">
                <StatusBadge :status="record.last_status" />
              </template>
              <template v-if="column.dataIndex === 'last_run_at'">
                <span class="time-text">{{ record.last_run_at || '未运行' }}</span>
              </template>
            </template>
            <template #emptyText>
              <a-empty description="暂无同步任务" :image-style="{ height: '60px' }">
                <a-button type="primary" size="small" @click="router.push('/sync/new')">创建任务</a-button>
              </a-empty>
            </template>
          </a-table>
        </div>
      </div>

      <div class="content-card">
        <div class="card-header">
          <span class="card-title">
            <FolderOutlined style="margin-right: 8px; color: #52c41a;" />
            仓库列表
          </span>
          <router-link to="/repos">
            <a-button type="link" size="small">查看全部 <RightOutlined /></a-button>
          </router-link>
        </div>
        <div class="card-body">
          <a-table
            :columns="repoColumns"
            :data-source="recentRepos"
            :pagination="false"
            size="small"
            row-key="key"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.dataIndex === 'name'">
                <a class="task-link" @click="router.push(`/local-repos/${record.key}`)">{{ record.name }}</a>
              </template>
              <template v-if="column.dataIndex === 'platform'">
                <a-tag :color="platformColor(record.platform)">{{ platformLabel(record.platform) }}</a-tag>
              </template>
              <template v-if="column.dataIndex === 'status'">
                <StatusBadge :status="record.status" />
              </template>
            </template>
            <template #emptyText>
              <a-empty description="暂无仓库" :image-style="{ height: '60px' }">
                <a-button type="primary" size="small" @click="router.push('/repos')">添加仓库</a-button>
              </a-empty>
            </template>
          </a-table>
        </div>
      </div>
    </div>

    <!-- System Status -->
    <div class="system-status-section">
      <div class="section-title">
        <DashboardOutlined style="margin-right: 8px;" />
        系统状态
      </div>
      <div class="system-status-card">
        <div class="status-items">
          <div class="status-item">
            <div class="status-item-label">
              <CheckCircleOutlined v-if="systemStatus?.status === 'running'" style="color: #52c41a; margin-right: 6px;" />
              <CloseCircleOutlined v-else style="color: #ff4d4f; margin-right: 6px;" />
              服务状态
            </div>
            <a-tag :color="systemStatus?.status === 'running' ? 'success' : 'error'">
              {{ systemStatus?.status === 'running' ? '运行中' : '已停止' }}
            </a-tag>
          </div>
          <a-divider type="vertical" style="height: 40px;" />
          <div class="status-item">
            <div class="status-item-label">
              <ClockCircleOutlined style="color: #1677ff; margin-right: 6px;" />
              Go 版本
            </div>
            <span class="status-value">{{ systemStatus?.go_version || '-' }}</span>
          </div>
          <a-divider type="vertical" style="height: 40px;" />
          <div class="status-item">
            <div class="status-item-label">
              <InfoCircleOutlined style="color: #722ed1; margin-right: 6px;" />
              版本号
            </div>
            <a-tag color="blue">{{ systemStatus?.version || 'v0.0.0' }}</a-tag>
          </div>
          <a-divider type="vertical" style="height: 40px;" />
          <div class="status-item">
            <div class="status-item-label">
              <CloudServerOutlined style="color: #faad14; margin-right: 6px;" />
              运行时长
            </div>
            <span class="status-value">{{ formatUptime(systemStatus?.uptime) }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
defineOptions({ name: 'Dashboard' })

import { computed, onMounted, onActivated, ref, markRaw } from 'vue'
import { useRouter } from 'vue-router'
import { useRepoStore } from '@/stores/repo'
import { useSyncTaskStore } from '@/stores/syncTask'
import { systemApi, repoApi } from '@/api'
import type { SystemStatusData } from '@/types/api'
import type { Repo } from '@/types'
import { notifyError } from '@/utils/notify'
import { platformLabel, platformColor } from '@/utils/platform'
import StatusBadge from '@/components/common/StatusBadge.vue'
import {
  FolderOutlined,
  SyncOutlined,
  PlayCircleOutlined,
  CloseCircleOutlined,
  PlusOutlined,
  FolderAddOutlined,
  RightOutlined,
  ArrowRightOutlined,
  ApiOutlined,
  HistoryOutlined,
  DashboardOutlined,
  ClockCircleOutlined,
  InfoCircleOutlined,
  CloudServerOutlined,
  CheckCircleOutlined,
} from '@ant-design/icons-vue'

const router = useRouter()
const repoStore = useRepoStore()
const taskStore = useSyncTaskStore()

const systemStatus = ref<SystemStatusData | null>(null)

// 本地数据,不复用 repoStore.repos:此前 Dashboard 拉前 5 条会覆盖共享 store,
// 导致切回仓库列表页(keep-alive)看到残缺列表
const dashRepos = ref<Repo[]>([])
const dashRepoTotal = ref(0)

const runningCount = computed(() => taskStore.tasks.filter((t) => t.last_status === 'running').length)
const failedCount = computed(() => taskStore.tasks.filter((t) => t.last_status === 'failed').length)
const recentTasks = computed(() => taskStore.tasks.slice(0, 5))
const recentRepos = computed(() => dashRepos.value.slice(0, 5))

const statCards = computed(() => [
  { label: '仓库总数', value: dashRepoTotal.value, icon: markRaw(FolderOutlined), color: 'blue', path: '/repos' },
  { label: '同步任务', value: taskStore.total, icon: markRaw(SyncOutlined), color: 'green', path: '/sync' },
  { label: '运行中', value: runningCount.value, icon: markRaw(PlayCircleOutlined), color: 'orange', path: '/sync' },
  { label: '失败任务', value: failedCount.value, icon: markRaw(CloseCircleOutlined), color: 'red', path: '/sync' },
])

const quickActions = [
  { label: '创建同步任务', icon: markRaw(SyncOutlined), color: 'blue', path: '/sync/new' },
  { label: '添加仓库', icon: markRaw(FolderAddOutlined), color: 'green', path: '/repos' },
  { label: '配置 Webhook', icon: markRaw(ApiOutlined), color: 'purple', path: '/webhook/rules' },
  { label: '查看日志', icon: markRaw(HistoryOutlined), color: 'cyan', path: '/sync/records' },
]

const formatUptime = (seconds?: number) => {
  if (!seconds) return '--'
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const mins = Math.floor((seconds % 3600) / 60)
  if (days > 0) return `${days}天 ${hours}小时`
  if (hours > 0) return `${hours}小时 ${mins}分钟`
  return `${mins}分钟`
}

const taskColumns = [
  { title: '任务名称', dataIndex: 'name', key: 'name', ellipsis: true },
  { title: '分支', dataIndex: 'branch', key: 'branch', width: 180 },
  { title: '状态', dataIndex: 'last_status', key: 'last_status', width: 90, align: 'center' as const },
  { title: '最后运行', dataIndex: 'last_run_at', key: 'last_run_at', width: 140 },
]

const repoColumns = [
  { title: '仓库名称', dataIndex: 'name', key: 'name', ellipsis: true },
  { title: '平台', dataIndex: 'platform', key: 'platform', width: 100, align: 'center' as const },
  { title: '状态', dataIndex: 'status', key: 'status', width: 90, align: 'center' as const },
]

async function fetchSystemStatus() {
  try {
    systemStatus.value = await systemApi.status()
  } catch (e) {
    notifyError(e, '获取系统状态失败')
  }
}

async function loadDashboard() {
  try {
    // Dashboard 只需总量 + 最近几条 + 状态统计;仓库数据进本地 ref,
    // 不写共享 store(避免污染仓库列表页的 keep-alive 缓存)
    const [repoData] = await Promise.all([
      repoApi.list({ page: 1, page_size: 5 }),
      taskStore.fetchTasks({ page: 1, page_size: 50 }),
      fetchSystemStatus(),
    ])
    dashRepos.value = repoData.list
    dashRepoTotal.value = repoData.pagination?.total ?? 0
  } catch (e) {
    notifyError(e, '加载仪表盘数据失败')
  }
}

// keep-alive 缓存页:首次挂载与每次切回都刷新
onMounted(loadDashboard)
onActivated(loadDashboard)
</script>

<style scoped lang="scss">
@use '@/styles/variables.scss' as *;

// 可点击卡片的交互态
.stat-card.clickable {
  cursor: pointer;
  border: 1px solid transparent;

  &:hover {
    border-color: #e6f7ff;

    .stat-arrow {
      opacity: 1;
      transform: translateX(0);
    }
  }
}

.stat-arrow {
  color: $text-tertiary;
  font-size: 12px;
  opacity: 0;
  transform: translateX(-4px);
  transition: all 0.2s ease;
}

/* 快捷操作 */
.quick-actions {
  margin-bottom: $spacing-lg;
}

.section-title {
  font-size: 15px;
  font-weight: 600;
  color: $text-primary;
  margin-bottom: $spacing-md;
}

.action-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: $spacing-md;
}

.action-item {
  background: $bg-primary;
  border-radius: $radius-md;
  padding: 16px 20px;
  display: flex;
  align-items: center;
  gap: 12px;
  cursor: pointer;
  transition: all 0.2s ease;
  box-shadow: $shadow-card;
  border: 1px solid transparent;

  &:hover {
    border-color: #e6f7ff;
    box-shadow: $shadow-card-hover;
    transform: translateY(-1px);
  }

  span {
    font-size: 14px;
    font-weight: 500;
    color: $text-primary;
  }
}

.action-icon {
  width: 36px;
  height: 36px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  flex-shrink: 0;

  &.blue   { background: #e6f7ff; color: $primary; }
  &.green  { background: #f6ffed; color: $success; }
  &.purple { background: #f9f0ff; color: #722ed1; }
  &.cyan   { background: #e6fffb; color: #13c2c2; }
}

/* 内容双栏 */
.grid-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: $spacing-md;
}

/* 系统状态 */
.system-status-section {
  margin-top: $spacing-lg;
}

.system-status-card {
  background: $bg-primary;
  border-radius: $radius-md;
  box-shadow: $shadow-card;
  padding: 20px $spacing-lg;
}

.status-items {
  display: flex;
  align-items: center;
  gap: $spacing-lg;
}

.status-item {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 140px;
}

.status-item-label {
  display: flex;
  align-items: center;
  font-size: 13px;
  color: $text-secondary;
}

.status-value {
  font-size: 14px;
  font-weight: 500;
  color: $text-primary;
}

@media (max-width: 1200px) {
  .action-grid {
    grid-template-columns: repeat(2, 1fr);
  }

  .grid-row {
    grid-template-columns: 1fr;
  }

  .status-items {
    flex-wrap: wrap;
    gap: $spacing-md;
  }
}

@media (max-width: 768px) {
  .status-items {
    flex-direction: column;
    align-items: flex-start;
  }

  .status-items :deep(.ant-divider-vertical) {
    display: none;
  }
}
</style>
