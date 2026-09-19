<template>
  <div class="page-container">
    <div class="page-header-bar">
      <div>
        <h1 class="page-title">操作日志</h1>
        <p class="page-subtitle">记录用户的登录、操作、配置变更等行为</p>
      </div>
      <a-space>
        <a-button @click="handleExport">
          <template #icon><DownloadOutlined /></template>
          导出日志
        </a-button>
        <a-button @click="fetchLogs">
          <template #icon><ReloadOutlined /></template>
          刷新
        </a-button>
      </a-space>
    </div>

    <!-- Stats Cards -->
    <div class="stats-row">
      <div class="stat-card">
        <div class="stat-icon" style="background: #e6f7ff; color: #1677ff;">
          <CalendarOutlined />
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ stats.today }}</div>
          <div class="stat-label">今日操作</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background: #f6ffed; color: #52c41a;">
          <CalendarOutlined />
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ stats.week }}</div>
          <div class="stat-label">本周操作</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background: #fff7e6; color: #faad14;">
          <FileTextOutlined />
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ stats.total }}</div>
          <div class="stat-label">总操作数</div>
        </div>
      </div>
    </div>

    <!-- Filter Bar -->
    <div class="filter-bar carded">
      <a-input
        v-model:value="filters.search"
        placeholder="搜索日志内容..."
        allow-clear
        style="width: 220px"
        @pressEnter="fetchLogs"
      >
        <template #prefix><SearchOutlined /></template>
      </a-input>
      <a-select
        v-model:value="filters.action"
        placeholder="操作类型"
        allow-clear
        style="width: 140px"
        @change="fetchLogs"
      >
        <a-select-option value="">全部类型</a-select-option>
        <a-select-option value="create">创建</a-select-option>
        <a-select-option value="update">更新</a-select-option>
        <a-select-option value="delete">删除</a-select-option>
        <a-select-option value="run">手动触发</a-select-option>
        <a-select-option value="retry">重试</a-select-option>
        <a-select-option value="sync">同步</a-select-option>
      </a-select>
      <a-range-picker
        v-model:value="filters.dateRange"
        :placeholder="['开始日期', '结束日期']"
        style="width: 260px"
        @change="handleDateChange"
      />
      <a-button type="primary" @click="fetchLogs">
        <template #icon><SearchOutlined /></template>
        查询
      </a-button>
      <a-button @click="resetFilters">
        <template #icon><UndoOutlined /></template>
        重置
      </a-button>
    </div>

    <!-- Log Table -->
    <a-table
      :columns="columns"
      :data-source="logs"
      :loading="loading"
      row-key="id"
      :scroll="{ x: 900 }"
      :pagination="pagination"
      @change="handleTableChange"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'time'">
          <span class="log-time">{{ formatDate(record.time) }}</span>
        </template>

        <template v-if="column.key === 'user'">
          <a-tag color="blue">{{ record.user }}</a-tag>
        </template>

        <template v-if="column.key === 'action'">
          <a-tag :color="actionColor(record.action)">
            {{ actionLabel(record.action) }}
          </a-tag>
        </template>

        <template v-if="column.key === 'resource'">
          <span class="log-resource">{{ record.resource }}</span>
        </template>

        <template v-if="column.key === 'ip'">
          <span class="log-ip">{{ record.ip }}</span>
        </template>
      </template>

      <template #emptyText>
        <a-empty description="暂无操作日志" />
      </template>
    </a-table>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref, reactive } from 'vue'
import type { TablePaginationConfig } from 'ant-design-vue'
import {
  ReloadOutlined,
  SearchOutlined,
  DownloadOutlined,
  CalendarOutlined,
  FileTextOutlined,
  UndoOutlined,
} from '@ant-design/icons-vue'
import { logApi } from '@/api'
import type { OperationLog, OperationLogParams } from '@/types/api'
import { notifyError, notifySuccess } from '@/utils/notify'
import dayjs, { type Dayjs } from 'dayjs'
import { formatDate } from '@/utils'

const loading = ref(false)
const logs = ref<OperationLog[]>([])
const stats = reactive({ today: 0, week: 0, total: 0 })

const filters = reactive({
  search: '',
  action: undefined as string | undefined,
  dateRange: null as [Dayjs, Dayjs] | null,
  startDate: '',
  endDate: '',
})

const pagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  showTotal: (total: number) => `共 ${total} 条日志`,
})

const columns = [
  { title: '时间', key: 'time', width: 180 },
  { title: '用户', key: 'user', width: 120 },
  { title: '操作类型', key: 'action', width: 100, align: 'center' as const },
  { title: '操作内容', key: 'resource', ellipsis: true },
  { title: 'IP', key: 'ip', width: 140 },
]

const actionColors: Record<string, string> = {
  login: 'blue',
  create: 'green',
  update: 'orange',
  delete: 'red',
  run: 'purple',
  retry: 'cyan',
  sync: 'geekblue',
}

const actionLabels: Record<string, string> = {
  login: '登录',
  create: '创建',
  update: '更新',
  delete: '删除',
  run: '手动触发',
  retry: '重试',
  sync: '同步',
}

function actionColor(action: string): string {
  return actionColors[action] || 'default'
}

function actionLabel(action: string): string {
  return actionLabels[action] || action
}


function handleDateChange(dates: [Dayjs, Dayjs] | null) {
  if (dates) {
    filters.startDate = dates[0].format('YYYY-MM-DD')
    filters.endDate = dates[1].format('YYYY-MM-DD')
  } else {
    filters.startDate = ''
    filters.endDate = ''
  }
}

function resetFilters() {
  filters.search = ''
  filters.action = undefined
  filters.dateRange = null
  filters.startDate = ''
  filters.endDate = ''
  pagination.current = 1
  fetchLogs()
}

function handleTableChange(pag: TablePaginationConfig) {
  pagination.current = pag.current || 1
  pagination.pageSize = pag.pageSize || 10
  fetchLogs()
}

async function fetchLogs() {
  loading.value = true
  try {
    const params: OperationLogParams = {
      page: pagination.current,
      page_size: pagination.pageSize,
    }
    if (filters.search) params.search = filters.search
    if (filters.action) params.action = filters.action
    if (filters.startDate) params.start_date = filters.startDate
    if (filters.endDate) params.end_date = filters.endDate

    const data = await logApi.listOperations(params)
    logs.value = data.list || []
    pagination.total = data.pagination?.total || 0
    stats.today = data.stats?.today ?? 0
    stats.week = data.stats?.week ?? 0
    stats.total = data.stats?.total ?? pagination.total
  } catch (e) {
    // 接口不可用时如实清空,不展示假数据
    logs.value = []
    pagination.total = 0
    stats.today = 0
    stats.week = 0
    stats.total = 0
    notifyError(e, '获取操作日志失败')
  } finally {
    loading.value = false
  }
}

// CSV 字段转义:含逗号/引号/换行的值用引号包裹并把内部引号翻倍,防列错位与公式注入
function csvEscape(v: string): string {
  const s = v ?? ''
  return /[",\n\r]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s
}

function handleExport() {
  const headers = ['时间', '用户', '操作类型', '操作内容', 'IP']
  const rows = logs.value.map((log) => [
    formatDate(log.time),
    log.user,
    actionLabel(log.action),
    log.resource,
    log.ip,
  ])
  const csv = [csvEscape(headers.join(',')), ...rows.map((r) => r.map(csvEscape).join(','))].join('\n')
  const blob = new Blob(['\uFEFF' + csv], { type: 'text/csv;charset=utf-8;' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `operation-logs-${dayjs().format('YYYY-MM-DD')}.csv`
  link.click()
  URL.revokeObjectURL(url)
  notifySuccess('日志导出成功')
}

onMounted(() => {
  fetchLogs()
})
</script>

<style scoped lang="scss">
@use '@/styles/variables.scss' as *;

// 本页统计卡为 3 列
.stats-row {
  grid-template-columns: repeat(3, 1fr);
}

// 过滤栏带卡片背景(覆盖全局)
.filter-bar.carded {
  padding: $spacing-md;
  background: $bg-primary;
  border-radius: $radius-md;
  box-shadow: $shadow-card;
}

.log-time {
  color: $text-secondary;
  font-size: 13px;
}

.log-resource {
  color: $text-primary;
}

.log-ip {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
  font-size: 13px;
  color: $text-secondary;
}
</style>
