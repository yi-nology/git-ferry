<template>
  <div class="page-container">
    <PageHeader title="仪表盘" subtitle="同步任务与仓库的运行概览">
      <template #actions>
        <a-button @click="router.push('/repos')">
          <template #icon><FolderAddOutlined /></template>
          添加仓库
        </a-button>
        <a-button type="primary" @click="router.push('/sync/new')">
          <template #icon><PlusOutlined /></template>
          新建同步任务
        </a-button>
      </template>
    </PageHeader>

    <!-- 关键指标：无彩色图标，失败一眼可见 -->
    <MetricStrip :items="metrics" />

    <!-- 待处理：失败任务优先于装饰性快捷入口 -->
    <div v-if="failedTasks.length" class="attention-card">
      <div class="attention-head">
        <AlertOutlined class="attention-icon" />
        <span>需要关注</span>
        <a-tag color="red" class="attention-count">{{ failedTasks.length }}</a-tag>
      </div>
      <ul class="attention-list">
        <li v-for="t in failedTasks" :key="t.key" class="attention-item">
          <span class="attention-name">{{ t.name }}</span>
          <span class="branch-tag">{{ t.source_branch }}</span>
          <ArrowRightOutlined class="attention-arrow" />
          <span class="branch-tag">{{ t.target_branch }}</span>
          <a-button type="link" size="small" @click="router.push('/sync')">处理</a-button>
        </li>
      </ul>
    </div>

    <!-- 内容双栏 -->
    <div class="grid-row">
      <section class="content-card">
        <div class="card-header">
          <span class="card-title">最近同步任务</span>
          <router-link to="/sync">
            <a-button type="link" size="small">查看全部</a-button>
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
                <ArrowRightOutlined class="cell-arrow" />
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
              <a-empty description="暂无同步任务" :image-style="{ height: '56px' }">
                <a-button type="primary" size="small" @click="router.push('/sync/new')">创建任务</a-button>
              </a-empty>
            </template>
          </a-table>
        </div>
      </section>

      <section class="content-card">
        <div class="card-header">
          <span class="card-title">最近仓库</span>
          <router-link to="/repos">
            <a-button type="link" size="small">查看全部</a-button>
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
                <a-tag>{{ platformLabel(record.platform) }}</a-tag>
              </template>
              <template v-if="column.dataIndex === 'status'">
                <StatusBadge :status="record.status" />
              </template>
            </template>
            <template #emptyText>
              <a-empty description="暂无仓库" :image-style="{ height: '56px' }">
                <a-button type="primary" size="small" @click="router.push('/repos')">添加仓库</a-button>
              </a-empty>
            </template>
          </a-table>
        </div>
      </section>
    </div>

    <!-- 系统状态：安静的元信息行 -->
    <section class="content-card system-card">
      <div class="card-header">
        <span class="card-title">系统状态</span>
      </div>
      <div class="card-body is-padded">
        <div class="system-meta">
          <div class="meta-item">
            <span class="meta-label">服务</span>
            <StatusBadge :status="systemStatus?.status === STATUS.Running ? 'success' : 'failed'" />
          </div>
          <div class="meta-item">
            <span class="meta-label">版本</span>
            <span class="meta-value">{{ systemStatus?.version || '-' }}</span>
          </div>
          <div class="meta-item">
            <span class="meta-label">Go</span>
            <span class="meta-value mono">{{ systemStatus?.go_version || '-' }}</span>
          </div>
          <div class="meta-item">
            <span class="meta-label">运行时长</span>
            <span class="meta-value">{{ formatUptime(systemStatus?.uptime) }}</span>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
defineOptions({ name: 'Dashboard' })

import { computed, onMounted, onActivated, ref } from 'vue'
import { useRouter } from 'vue-router'
import { syncTaskApi, systemApi, repoApi } from '@/api'
import type { SystemStatusData } from '@/types/api'
import type { Repo, SyncTask } from '@/types'
import { notifyError } from '@/utils/notify'
import { STATUS } from '@/constants/status'
import { platformLabel } from '@/utils/platform'
import StatusBadge from '@/components/common/StatusBadge.vue'
import PageHeader from '@/components/common/PageHeader.vue'
import MetricStrip, { type MetricItem } from '@/components/common/MetricStrip.vue'
import {
  PlusOutlined,
  FolderAddOutlined,
  ArrowRightOutlined,
  AlertOutlined,
} from '@ant-design/icons-vue'

const router = useRouter()

const systemStatus = ref<SystemStatusData | null>(null)

// 本地数据,不复用 repoStore.repos:此前 Dashboard 拉前 5 条会覆盖共享 store,
// 导致切回仓库列表页(keep-alive)看到残缺列表
const dashRepos = ref<Repo[]>([])
const dashRepoTotal = ref(0)

// 任务数据同样进本地 ref:taskStore.fetchTasks 会覆盖列表页共享缓存
const dashTasks = ref<SyncTask[]>([])
const dashTaskTotal = ref(0)

const runningCount = computed(() => dashTasks.value.filter((t) => t.last_status === STATUS.Running).length)
const failedTasks = computed(() => dashTasks.value.filter((t) => t.last_status === STATUS.Failed))
const recentTasks = computed(() => dashTasks.value.slice(0, 5))
const recentRepos = computed(() => dashRepos.value.slice(0, 5))

const metrics = computed<MetricItem[]>(() => [
  { label: '仓库', value: dashRepoTotal.value, path: '/repos' },
  { label: '同步任务', value: dashTaskTotal.value, path: '/sync' },
  { label: '运行中', value: runningCount.value, tone: 'info', path: '/sync' },
  {
    label: '失败',
    value: failedTasks.value.length,
    tone: failedTasks.value.length ? 'danger' : 'default',
    path: '/sync',
  },
])

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
  { title: '任务', dataIndex: 'name', key: 'name', ellipsis: true },
  { title: '分支', dataIndex: 'branch', key: 'branch', width: 200 },
  { title: '状态', dataIndex: 'last_status', key: 'last_status', width: 96, align: 'center' as const },
  { title: '最后运行', dataIndex: 'last_run_at', key: 'last_run_at', width: 150 },
]

const repoColumns = [
  { title: '仓库', dataIndex: 'name', key: 'name', ellipsis: true },
  { title: '平台', dataIndex: 'platform', key: 'platform', width: 110, align: 'center' as const },
  { title: '状态', dataIndex: 'status', key: 'status', width: 96, align: 'center' as const },
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
    const [repoData, taskData] = await Promise.all([
      repoApi.list({ page: 1, page_size: 5 }),
      syncTaskApi.list({ page: 1, page_size: 50 }),
      fetchSystemStatus(),
    ])
    dashRepos.value = repoData.list
    dashRepoTotal.value = repoData.pagination?.total ?? 0
    dashTasks.value = taskData.tasks || []
    dashTaskTotal.value = taskData.total ?? dashTasks.value.length
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

.cell-arrow {
  color: $text-tertiary;
  font-size: 11px;
  margin: 0 4px;
}

/* 需要关注 */
.attention-card {
  background: $error-soft;
  border: 1px solid $error-border;
  border-radius: $radius-lg;
  padding: 12px 16px;
  margin-bottom: $spacing-lg;
}

.attention-head {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: $fs-md;
  font-weight: 600;
  color: $error;
  margin-bottom: 8px;
}

.attention-icon {
  font-size: 14px;
}

.attention-count {
  margin-left: auto;
}

.attention-list {
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.attention-item {
  display: flex;
  align-items: center;
  gap: 8px;
  background: rgba(255, 255, 255, 0.7);
  border-radius: $radius-md;
  padding: 6px 10px;
  font-size: $fs-body;
}

.attention-name {
  font-weight: 500;
  color: $text-primary;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 280px;
}

.attention-arrow {
  color: $text-tertiary;
  font-size: 11px;
}

.attention-item :deep(.ant-btn-link) {
  margin-left: auto;
  padding-inline: 4px;
}

/* 内容双栏 */
.grid-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: $spacing-md;
  margin-bottom: $spacing-md;
}

/* 系统状态 */
.system-card {
  margin-top: 0;
}

.system-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 20px 32px;
  align-items: center;
}

.meta-item {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.meta-label {
  font-size: $fs-caption;
  color: $text-tertiary;
  font-weight: 500;
}

.meta-value {
  font-size: $fs-body;
  color: $text-primary;
  font-weight: 500;

  &.mono {
    font-family: $font-mono;
    font-size: 12px;
  }
}

@media (max-width: 1100px) {
  .grid-row {
    grid-template-columns: 1fr;
  }
}
</style>
