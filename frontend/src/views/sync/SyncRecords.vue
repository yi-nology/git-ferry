<template>
  <div class="page-container">
    <div class="page-header-bar">
      <div>
        <h1 class="page-title">执行记录</h1>
        <p class="page-subtitle">查看所有同步任务的执行历史和详细日志</p>
      </div>
      <a-space>
        <a-button @click="fetchRecords">
          <template #icon><ReloadOutlined /></template>
          刷新
        </a-button>
      </a-space>
    </div>

    <!-- 统计概览 -->
    <div class="stats-row">
      <div class="stat-card">
        <div class="stat-icon" style="background: #e6f7ff; color: #1677ff;">
          <ThunderboltOutlined />
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ stats.today }}</div>
          <div class="stat-label">今日执行</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background: #f6ffed; color: #52c41a;">
          <CheckCircleOutlined />
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ stats.successRate }}</div>
          <div class="stat-label">成功率</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background: #fff2f0; color: #ff4d4f;">
          <CloseCircleOutlined />
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ stats.failed }}</div>
          <div class="stat-label">失败</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background: #fff7e6; color: #faad14;">
          <ClockCircleOutlined />
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ stats.avgDuration }}</div>
          <div class="stat-label">平均耗时</div>
        </div>
      </div>
    </div>

    <!-- 筛选栏 -->
    <div class="filter-bar">
      <a-select
        v-model:value="filters.taskKey"
        placeholder="选择任务"
        allow-clear
        show-search
        :filter-option="filterTaskOption"
        style="width: 200px"
      >
        <a-select-option value="">全部任务</a-select-option>
        <a-select-option v-for="task in tasks" :key="task.key" :value="task.key">
          {{ task.name }}
        </a-select-option>
      </a-select>
      <a-select
        v-model:value="filters.status"
        placeholder="执行状态"
        allow-clear
        style="width: 140px"
      >
        <a-select-option value="">全部状态</a-select-option>
        <a-select-option value="success">成功</a-select-option>
        <a-select-option value="failed">失败</a-select-option>
        <a-select-option value="running">运行中</a-select-option>
      </a-select>
      <a-select
        v-model:value="filters.trigger"
        placeholder="触发方式"
        allow-clear
        style="width: 140px"
      >
        <a-select-option value="">全部方式</a-select-option>
        <a-select-option value="manual">手动触发</a-select-option>
        <a-select-option value="cron">定时任务</a-select-option>
        <a-select-option value="webhook">Webhook</a-select-option>
        <a-select-option value="retry">重试</a-select-option>
      </a-select>
      <a-range-picker
        v-model:value="filters.dateRange"
        :placeholder="['开始日期', '结束日期']"
        style="width: 260px"
      />
      <a-button type="primary" @click="fetchRecords">
        <template #icon><SearchOutlined /></template>
        查询
      </a-button>
      <a-button @click="resetFilters">
        <template #icon><UndoOutlined /></template>
        重置
      </a-button>
    </div>

    <!-- 记录表格 -->
    <a-table
      :columns="columns"
      :data-source="filteredRecords"
      :loading="loading"
      row-key="id"
      :scroll="{ x: 1000 }"
      :pagination="pagination"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'time'">
          <div class="time-cell">
            <div>{{ formatDate(record.start_time) }}</div>
            <div v-if="record.end_time" class="time-end">→ {{ formatDate(record.end_time) }}</div>
          </div>
        </template>

        <template v-if="column.key === 'task'">
          <a
            class="task-link"
            role="button"
            tabindex="0"
            @click="openDrawer(record)"
            @keydown.enter="openDrawer(record)"
          >{{ taskNameOf(record.task_key) }}</a>
        </template>

        <template v-if="column.key === 'flow'">
          <div class="flow-cell">
            <a-tooltip :title="sourceRepoOf(record.task_key)">
              <span class="flow-repo">{{ repoShort(sourceRepoOf(record.task_key)) }}</span>
            </a-tooltip>
            <span class="flow-branch">{{ sourceBranchOf(record.task_key) }}</span>
            <ArrowRightOutlined class="flow-arrow" />
            <a-tooltip :title="targetRepoOf(record.task_key)">
              <span class="flow-repo">{{ repoShort(targetRepoOf(record.task_key)) }}</span>
            </a-tooltip>
            <span class="flow-branch">{{ targetBranchOf(record.task_key) }}</span>
          </div>
        </template>

        <template v-if="column.key === 'trigger'">
          <a-tag :color="triggerColor(record.trigger_source)">
            {{ triggerLabel(record.trigger_source) }}
          </a-tag>
        </template>

        <template v-if="column.key === 'status'">
          <StatusBadge :status="record.status" />
        </template>

        <template v-if="column.key === 'duration'">
          <span class="duration-text">{{ formatDuration(record.duration_ms) }}</span>
        </template>

        <template v-if="column.key === 'actions'">
          <a-space :size="0">
            <a-tooltip title="查看详情">
              <a-button type="text" size="small" @click="openDrawer(record)">
                <template #icon><EyeOutlined /></template>
              </a-button>
            </a-tooltip>
            <a-tooltip title="重新运行">
              <a-button
                type="text"
                size="small"
                :loading="runningKeys[record.task_key]"
                @click="rerun(record)"
              >
                <template #icon><PlayCircleOutlined /></template>
              </a-button>
            </a-tooltip>
            <a-popconfirm
              title="确定删除这条执行记录?"
              ok-text="删除"
              cancel-text="取消"
              @confirm="removeRecord(record)"
            >
              <a-tooltip title="删除记录">
                <a-button type="text" size="small" danger>
                  <template #icon><DeleteOutlined /></template>
                </a-button>
              </a-tooltip>
            </a-popconfirm>
          </a-space>
        </template>
      </template>

      <template #emptyText>
        <a-empty description="暂无执行记录">
          <template #description>
            <span>{{ filters.taskKey ? '该任务暂无执行记录' : '暂无同步执行记录' }}</span>
          </template>
        </a-empty>
      </template>
    </a-table>

    <!-- 详情抽屉 -->
    <a-drawer
      v-model:open="drawerVisible"
      title="执行详情"
      :width="640"
      placement="right"
    >
      <template v-if="currentRecord">
        <a-descriptions :column="2" bordered size="small" class="detail-section">
          <a-descriptions-item label="任务名称" :span="2">
            {{ taskNameOf(currentRecord.task_key) }}
          </a-descriptions-item>
          <a-descriptions-item label="触发方式">
            <a-tag :color="triggerColor(currentRecord.trigger_source)">
              {{ triggerLabel(currentRecord.trigger_source) }}
            </a-tag>
          </a-descriptions-item>
          <a-descriptions-item label="执行状态">
            <StatusBadge :status="currentRecord.status" />
          </a-descriptions-item>
          <a-descriptions-item label="开始时间">
            {{ formatDate(currentRecord.start_time) }}
          </a-descriptions-item>
          <a-descriptions-item label="结束时间">
            {{ formatDate(currentRecord.end_time) }}
          </a-descriptions-item>
          <a-descriptions-item label="执行耗时" :span="2">
            {{ formatDuration(currentRecord.duration_ms) }}
          </a-descriptions-item>
        </a-descriptions>

        <a-divider>同步信息</a-divider>
        <a-descriptions :column="2" bordered size="small" class="detail-section">
          <a-descriptions-item label="源仓库" :span="2">
            {{ sourceRepoOf(currentRecord.task_key) }}
          </a-descriptions-item>
          <a-descriptions-item label="源分支">
            <span class="branch-tag">{{ sourceBranchOf(currentRecord.task_key) }}</span>
          </a-descriptions-item>
          <a-descriptions-item label="目标分支">
            <span class="branch-tag">{{ targetBranchOf(currentRecord.task_key) }}</span>
          </a-descriptions-item>
          <a-descriptions-item label="提交范围" :span="2">
            <span class="commit-range">{{ currentRecord.commit_range || '-' }}</span>
          </a-descriptions-item>
        </a-descriptions>

        <a-divider>执行日志</a-divider>
        <div class="execution-log">
          <div v-if="parsedDetails.length" class="log-steps">
            <div v-for="(step, index) in parsedDetails" :key="index" class="log-step">
              <div class="step-marker">
                <div class="step-dot" :class="step.type || 'info'"></div>
                <div v-if="index < parsedDetails.length - 1" class="step-line"></div>
              </div>
              <div class="step-content">
                <div class="step-time">{{ step.time || '' }}</div>
                <div class="step-message">{{ step.message }}</div>
              </div>
            </div>
          </div>
          <div v-else-if="currentRecord.details" class="log-raw">
            <pre>{{ currentRecord.details }}</pre>
          </div>
          <a-empty v-else :image="simpleImage" description="暂无执行日志" />
        </div>

        <template v-if="currentRecord.status === 'failed' && currentRecord.error_message">
          <a-divider>错误信息</a-divider>
          <a-alert
            type="error"
            :message="currentRecord.error_message"
            show-icon
            class="error-alert"
          />
        </template>

        <div class="drawer-actions">
          <a-space>
            <a-button
              v-if="currentRecord.status === 'failed'"
              type="primary"
              danger
              @click="rerun(currentRecord)"
            >
              <template #icon><RedoOutlined /></template>
              重试任务
            </a-button>
            <a-button @click="drawerVisible = false">关闭</a-button>
          </a-space>
        </div>
      </template>
    </a-drawer>
  </div>
</template>

<script setup lang="ts">
defineOptions({ name: 'SyncRecords' })
import { ref, reactive, computed, onMounted, onActivated } from 'vue'
import { Empty } from 'ant-design-vue'
import {
  ReloadOutlined,
  SearchOutlined,
  UndoOutlined,
  EyeOutlined,
  PlayCircleOutlined,
  DeleteOutlined,
  ArrowRightOutlined,
  ThunderboltOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  ClockCircleOutlined,
  RedoOutlined,
} from '@ant-design/icons-vue'
import { useSyncTaskStore } from '@/stores/syncTask'
import { syncTaskApi } from '@/api'
import type { SyncRun } from '@/types'
import StatusBadge from '@/components/common/StatusBadge.vue'
import { triggerColor, triggerLabel } from '@/utils/dictionaries'
import { notifyError, notifySuccess } from '@/utils/notify'
import dayjs, { type Dayjs } from 'dayjs'
import { formatDate } from '@/utils'

const simpleImage = Empty.PRESENTED_IMAGE_SIMPLE

const taskStore = useSyncTaskStore()
const loading = ref(false)
const allRecords = ref<SyncRun[]>([])
const drawerVisible = ref(false)
const currentRecord = ref<SyncRun | null>(null)
const runningKeys = ref<Record<string, boolean>>({})

const filters = reactive({
  taskKey: '' as string,
  status: undefined as string | undefined,
  trigger: undefined as string | undefined,
  dateRange: null as [Dayjs, Dayjs] | null,
})

const columns = [
  { title: '时间', key: 'time', width: 200 },
  { title: '任务名称', key: 'task', width: 160, ellipsis: true },
  { title: '源 → 目标', key: 'flow', minWidth: 260 },
  { title: '触发方式', key: 'trigger', width: 100, align: 'center' as const },
  { title: '状态', key: 'status', width: 90, align: 'center' as const },
  { title: '耗时', key: 'duration', width: 100 },
  { title: '操作', key: 'actions', width: 120, align: 'center' as const },
]

const pagination = reactive({
  current: 1,
  pageSize: 20,
  total: 0,
  showSizeChanger: true,
  showTotal: (total: number) => `共 ${total} 条记录`,
  onChange: (page: number, size: number) => {
    pagination.current = page
    pagination.pageSize = size
  },
})

// 任务映射表
const tasks = computed(() => taskStore.tasks)

const taskMap = computed(() => {
  const map: Record<string, any> = {}
  for (const t of taskStore.tasks) map[t.key] = t
  return map
})

function taskNameOf(key: string) {
  return taskMap.value[key]?.name || key
}
function sourceRepoOf(key: string) {
  return taskMap.value[key]?.source_repo_key || '-'
}
function targetRepoOf(key: string) {
  return taskMap.value[key]?.target_repo_key || '-'
}
function sourceBranchOf(key: string) {
  return taskMap.value[key]?.source_branch || ''
}
function targetBranchOf(key: string) {
  return taskMap.value[key]?.target_branch || ''
}
function repoShort(full: string) {
  if (!full || full === '-') return '-'
  const parts = full.split('/')
  return parts[parts.length - 1] || full
}


function formatDuration(ms?: number) {
  if (!ms || ms <= 0) return '-'
  if (ms < 1000) return `${ms}ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  return `${Math.floor(ms / 60000)}m ${Math.floor((ms % 60000) / 1000)}s`
}

function filterTaskOption(input: string, option: any) {
  if (!option.value) return true
  const task = taskMap.value[option.value]
  return task?.name?.toLowerCase().includes(input.toLowerCase()) || false
}

// 过滤后的记录
const filteredRecords = computed(() => {
  let rows = allRecords.value

  if (filters.taskKey) {
    rows = rows.filter((r) => r.task_key === filters.taskKey)
  }
  if (filters.status) {
    rows = rows.filter((r) => r.status === filters.status)
  }
  if (filters.trigger) {
    rows = rows.filter((r) => r.trigger_source === filters.trigger)
  }
  if (filters.dateRange) {
    const [start, end] = filters.dateRange
    rows = rows.filter((r) => {
      if (!r.start_time) return false
      const t = dayjs(r.start_time)
      return t.isAfter(start.startOf('day')) && t.isBefore(end.endOf('day'))
    })
  }

  pagination.total = rows.length
  return rows
})

// 统计数据
const stats = computed(() => {
  const rows = filteredRecords.value
  const total = rows.length
  const today = dayjs().format('YYYY-MM-DD')
  const todayCount = rows.filter((r) => r.start_time?.startsWith(today)).length
  const success = rows.filter((r) => r.status === 'success').length
  const failed = rows.filter((r) => r.status === 'failed').length

  const durations = rows
    .filter((r) => r.start_time && r.end_time)
    .map((r) => new Date(r.end_time).getTime() - new Date(r.start_time).getTime())
    .filter((d) => d > 0)
  const avg = durations.length
    ? durations.reduce((a, b) => a + b, 0) / durations.length
    : 0

  return {
    today: todayCount,
    successRate: total ? `${Math.round((success / total) * 100)}%` : '-',
    failed,
    avgDuration: avg ? formatDuration(Math.round(avg)) : '-',
  }
})

// 解析执行详情为结构化步骤
const parsedDetails = computed(() => {
  if (!currentRecord.value?.details) return []
  try {
    const parsed = JSON.parse(currentRecord.value.details)
    if (Array.isArray(parsed)) return parsed as { time?: string; message: string; type?: string }[]
    return []
  } catch {
    const lines = currentRecord.value.details.split('\n').filter((l) => l.trim())
    return lines.map((line) => {
      const timeMatch = line.match(/^\[([^\]]+)\]\s*(.*)$/)
      if (timeMatch) {
        return { time: timeMatch[1], message: timeMatch[2], type: 'info' }
      }
      return { message: line, type: 'info' }
    })
  }
})

function openDrawer(record: SyncRun) {
  currentRecord.value = record
  drawerVisible.value = true
}

async function rerun(record: SyncRun) {
  runningKeys.value[record.task_key] = true
  try {
    await syncTaskApi.run(record.task_key)
    notifySuccess('任务已触发,请稍后刷新查看结果')
  } catch (e) {
    notifyError(e, '触发任务失败')
  } finally {
    runningKeys.value[record.task_key] = false
  }
}

async function removeRecord(record: SyncRun) {
  try {
    await syncTaskApi.deleteHistory(record.id)
    notifySuccess('已删除该执行记录')
    fetchRecords()
  } catch (e) {
    notifyError(e, '删除失败')
  }
}

function resetFilters() {
  filters.taskKey = ''
  filters.status = undefined
  filters.trigger = undefined
  filters.dateRange = null
  pagination.current = 1
}

async function fetchRecords() {
  loading.value = true
  try {
    // 并行请求所有任务的执行记录,而非逐个串行等待
    const results = await Promise.allSettled(
      taskStore.tasks.map((task) =>
        syncTaskApi.history({ task_key: task.key, limit: 50 })
      ),
    )
    const allRuns: SyncRun[] = []
    for (const r of results) {
      if (r.status === 'fulfilled') {
        allRuns.push(...(r.value.runs || []))
      }
    }
    allRuns.sort(
      (a, b) => new Date(b.start_time || 0).getTime() - new Date(a.start_time || 0).getTime(),
    )
    allRecords.value = allRuns
  } catch (e) {
    notifyError(e, '加载执行记录失败')
    allRecords.value = []
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  try {
    await taskStore.fetchTasks()
  } catch (e) {
    notifyError(e, '加载任务失败')
  }
  fetchRecords()
})

// keep-alive 缓存页:每次切回刷新执行记录
onActivated(() => {
  fetchRecords()
})
</script>

<style scoped lang="scss">
@use '@/styles/variables.scss' as *;

.filter-bar {
  display: flex;
  align-items: center;
  gap: $spacing-md;
  margin-bottom: $spacing-lg;
  flex-wrap: wrap;
}

.task-link {
  font-weight: 500;
  color: $primary;
  cursor: pointer;

  &:hover {
    text-decoration: underline;
  }
}

.time-cell {
  .time-end {
    color: $text-secondary;
    font-size: 12px;
  }
}

.flow-cell {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;

  .flow-repo {
    font-weight: 500;
    max-width: 120px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .flow-branch {
    color: $text-secondary;
    font-size: 12px;
  }

  .flow-arrow {
    color: $text-secondary;
    font-size: 12px;
  }
}

.duration-text {
  font-variant-numeric: tabular-nums;
}

/* 抽屉详情 */
.detail-section {
  margin-bottom: $spacing-md;
}

.execution-log {
  max-height: 400px;
  overflow-y: auto;
}

.log-steps {
  padding: $spacing-sm 0;
}

.log-step {
  display: flex;
  gap: $spacing-md;
  min-height: 40px;
}

.step-marker {
  display: flex;
  flex-direction: column;
  align-items: center;
  width: 16px;
  flex-shrink: 0;
}

.step-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  margin-top: 4px;
  flex-shrink: 0;

  &.info    { background: $primary; }
  &.success { background: $success; }
  &.error   { background: $error; }
  &.warning { background: $warning; }
}

.step-line {
  width: 2px;
  flex: 1;
  background: $border;
  margin: 4px 0;
}

.step-content {
  flex: 1;
  padding-bottom: $spacing-md;
}

.step-time {
  font-size: 12px;
  color: $text-secondary;
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
}

.step-message {
  font-size: 13px;
  color: $text-primary;
  margin-top: 2px;
  line-height: 1.5;
}

.log-raw {
  pre {
    background: #fafafa;
    border: 1px solid $border;
    border-radius: $radius-sm;
    padding: $spacing-md;
    font-size: 12px;
    font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
    color: $text-primary;
    white-space: pre-wrap;
    word-break: break-all;
    max-height: 300px;
    overflow-y: auto;
  }
}

.error-alert {
  margin-bottom: $spacing-md;
}

.drawer-actions {
  margin-top: $spacing-lg;
  padding-top: $spacing-md;
  border-top: 1px solid $border;
  display: flex;
  justify-content: flex-end;
}

.branch-tag {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
  font-size: 13px;
}

.commit-range {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
  font-size: 12px;
  color: $text-secondary;
}

:deep(.ant-descriptions-item-label) {
  font-weight: 500;
  color: $text-secondary;
  width: 100px;
}

@media (max-width: 768px) {
  .filter-bar {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
