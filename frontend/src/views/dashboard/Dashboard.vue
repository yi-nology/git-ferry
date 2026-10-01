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

    <!-- 运维待办队列：可复制动作 -->
    <div v-if="todoItems.length" class="attention-card todo-card">
      <div class="attention-head">
        <OrderedListOutlined class="attention-icon" />
        <span>运维待办</span>
        <a-tag color="orange" class="attention-count">{{ todoItems.length }}</a-tag>
        <a-button type="link" size="small" @click="router.push('/ops')">运维中心</a-button>
      </div>
      <ul class="todo-list">
        <li v-for="item in todoItems" :key="item.id" class="todo-item">
          <a-tag :color="item.priority === 1 ? 'red' : 'orange'">P{{ item.priority }}</a-tag>
          <a-tag>{{ kindLabel(item.kind) }}</a-tag>
          <span class="todo-title">{{ item.title }}</span>
          <span v-if="item.reason" class="todo-reason">{{ item.reason }}</span>
          <div class="todo-actions">
            <template v-for="(a, i) in (item.actions || []).slice(0, 2)" :key="i">
              <a-tooltip :title="a.command || a.title">
                <a-button size="small" :danger="!!a.danger" @click="copyAction(a)">
                  <template #icon><CopyOutlined /></template>
                  {{ a.title }}
                  <span v-if="a.danger" class="danger-dot" title="危险命令，执行需确认">⚠</span>
                </a-button>
              </a-tooltip>
            </template>
          </div>
        </li>
      </ul>
    </div>

    <!-- 运维健康：一眼看清整体水位 -->
    <section v-if="healthSummary" class="content-card health-card">
      <div class="card-header">
        <span class="card-title">运维健康</span>
        <router-link to="/ops">
          <a-button type="link" size="small">运维中心</a-button>
        </router-link>
      </div>
      <div class="card-body is-padded">
        <div class="health-row">
          <div class="health-score">
            <span class="health-score-num">{{ healthSummary.avg }}</span>
            <span class="health-score-label">平均分</span>
          </div>
          <div class="health-levels">
            <div
              v-for="lv in healthSummary.levels"
              :key="lv.key"
              class="level-chip"
              :class="lv.key"
            >
              <span class="level-dot" />
              <span class="level-name">{{ lv.label }}</span>
              <span class="level-count">{{ lv.count }}</span>
            </div>
          </div>
          <div class="health-issues">
            <template v-if="healthSummary.issueCount > 0">
              <WarningOutlined class="issue-icon" />
              <span>{{ healthSummary.issueCount }} 项待改进</span>
            </template>
            <span v-else class="health-ok">未发现明显问题</span>
          </div>
        </div>
        <div v-if="healthSummary.weakDims.length" class="weak-dims">
          <span class="weak-label">薄弱维度</span>
          <a-tag v-for="w in healthSummary.weakDims" :key="w.name" :color="w.count > 1 ? 'red' : 'orange'">
            {{ w.name }} ×{{ w.count }}
          </a-tag>
          <router-link to="/ops" class="weak-link">去运维中心处理 →</router-link>
        </div>
      </div>
    </section>

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
import { syncTaskApi, systemApi, repoApi, opsApi } from '@/api'
import type { SystemStatusData } from '@/types/api'
import type { Repo, SyncTask } from '@/types'
import type { HealthScoreItem, OpsTodoAction, OpsTodoItem } from '@/api/ops'
import { notifyError, notifySuccess } from '@/utils/notify'
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
  WarningOutlined,
  CopyOutlined,
  OrderedListOutlined,
} from '@ant-design/icons-vue'
import { copyToClipboard } from '@/utils'

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

const healthItems = ref<HealthScoreItem[]>([])
const todoItems = ref<OpsTodoItem[]>([])

function kindLabel(k: string) {
  return { health: '健康', orphan: '孤儿仓', rpo: 'RPO', drift: '漂移' }[k] || k
}

async function copyAction(a: OpsTodoAction) {
  const text = a.command || a.title
  try {
    await copyToClipboard(text)
    notifySuccess(a.command ? '命令已复制' : '已复制')
  } catch {
    notifyError('复制失败')
  }
}

const healthSummary = computed(() => {
  if (!healthItems.value.length) return null
  const levels = [
    { key: 'gold', label: '金牌', count: 0 },
    { key: 'silver', label: '银牌', count: 0 },
    { key: 'bronze', label: '铜牌', count: 0 },
    { key: 'basic', label: '基础', count: 0 },
  ] as const
  const counts: Record<string, number> = { gold: 0, silver: 0, bronze: 0, basic: 0 }
  let sum = 0
  let issueCount = 0
  const dimCount: Record<string, number> = {}
  for (const it of healthItems.value) {
    counts[it.level] = (counts[it.level] || 0) + 1
    sum += it.score || 0
    if (it.issues?.length) issueCount += it.issues.length
    for (const d of it.dimensions || []) {
      if ((d.score ?? 100) < 60) {
        dimCount[d.name] = (dimCount[d.name] || 0) + 1
      }
    }
  }
  const weakDims = Object.entries(dimCount)
    .map(([name, count]) => ({ name, count }))
    .sort((a, b) => b.count - a.count || a.name.localeCompare(b.name))
    .slice(0, 5)
  return {
    avg: Math.round(sum / healthItems.value.length),
    levels: levels.map((l) => ({ ...l, count: counts[l.key] || 0 })),
    issueCount,
    weakDims,
  }
})

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

  // 健康评分独立拉取:失败不影响主内容
  try {
    const data = await opsApi.healthScore(50)
    healthItems.value = data.items || []
  } catch {
    healthItems.value = []
  }

  // 运维待办队列
  try {
    const todo = await opsApi.opsTodo()
    todoItems.value = (todo.items || []).slice(0, 8)
  } catch {
    todoItems.value = []
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

/* 运维待办 */
.todo-card {
  margin-top: $spacing-md;
}

.todo-list {
  list-style: none;
  padding: 0;
  margin: 0;
}

.todo-item {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding: 8px 0;
  border-bottom: 1px dashed $border-muted;

  &:last-child {
    border-bottom: none;
  }
}

.todo-title {
  font-weight: 500;
  color: $text-primary;
  font-size: $fs-body;
}

.todo-reason {
  color: $text-tertiary;
  font-size: $fs-caption;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 280px;
}

.todo-actions {
  margin-left: auto;
  display: flex;
  gap: 6px;
  flex-wrap: wrap;

  .danger-dot {
    margin-left: 2px;
    color: $warning;
    font-size: 11px;
  }
}

.attention-item :deep(.ant-btn-link) {
  margin-left: auto;
  padding-inline: 4px;
}

/* 运维健康 */
.health-card {
  margin-bottom: $spacing-md;
}

.health-row {
  display: flex;
  align-items: center;
  gap: 20px;
  flex-wrap: wrap;
}

.health-score {
  display: flex;
  align-items: baseline;
  gap: 6px;
  padding-right: 16px;
  border-right: 1px solid $border-muted;
}

.health-score-num {
  font-size: 28px;
  font-weight: 600;
  letter-spacing: -0.5px;
  font-variant-numeric: tabular-nums;
  color: $text-primary;
}

.health-score-label {
  font-size: $fs-caption;
  color: $text-tertiary;
}

.health-levels {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.level-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  border-radius: 999px;
  border: 1px solid $border-light;
  background: $bg-subtle;
  font-size: $fs-caption;
  color: $text-secondary;

  .level-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: $text-tertiary;
  }

  .level-count {
    font-weight: 600;
    color: $text-primary;
    font-variant-numeric: tabular-nums;
  }

  &.gold {
    background: #fff8c5;
    border-color: #eed888;
    .level-dot { background: #9a6700; }
  }
  &.silver {
    background: #f6f8fa;
    border-color: $border;
    .level-dot { background: #656d76; }
  }
  &.bronze {
    background: #ffebe9;
    border-color: #ffcecb;
    .level-dot { background: #cf222e; }
  }
  &.basic {
    background: $bg-subtle;
    .level-dot { background: $text-tertiary; }
  }
}

.health-issues {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: $fs-body;
  color: $warning;

  .issue-icon {
    font-size: 13px;
  }

  .health-ok {
    color: $success;
  }
}

.weak-dims {
  margin-top: 10px;
  padding-top: 10px;
  border-top: 1px dashed $border-muted;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;

  .weak-label {
    font-size: $fs-caption;
    color: $text-tertiary;
  }

  .weak-link {
    margin-left: auto;
    font-size: $fs-caption;
  }
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
